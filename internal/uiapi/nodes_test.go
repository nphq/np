package uiapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

func TestNodesServiceValidation(t *testing.T) {
	pool := cluster.NewPool(config.New(t.TempDir()+"/c.json"), secure.NewMemory())
	loads := NewLoadsService(pool)
	svc := NewNodesService(pool, loads)
	if _, e := svc.ListNodes(context.Background(), "bad id!"); e == nil || e.Code != CodeInvalidInput {
		t.Fatalf("want invalid_input, got %+v", e)
	}
}

func TestNodesServiceMergesUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes":
			_, _ = w.Write([]byte(`[{"ID":"n1","Name":"node-1","Status":"ready","NodeResources":{"Cpu":{"CpuShares":3200,"TotalCpuCores":4},"Memory":{"MemoryMB":8192},"Disk":{"DiskMB":100000}}}]`))
		case "/v1/allocations":
			_, _ = w.Write([]byte(`[]`))
		case "/v1/client/stats":
			_, _ = w.Write([]byte(`{"Memory":{"Total":8589934592,"Used":1073741824},"CPU":[{"CPU":"cpu0","Idle":50}],"DiskStats":[{"Used":52428800}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	store := config.New(t.TempDir() + "/clusters.json")
	if err := store.Add(&config.ClusterConfig{ID: "dev", Name: "Dev", Address: srv.URL}); err != nil {
		t.Fatal(err)
	}
	pool := cluster.NewPool(store, secure.NewMemory())
	loads := NewLoadsService(pool)
	svc := NewNodesService(pool, loads)

	nodes, e := svc.ListNodes(context.Background(), "dev")
	if e != nil {
		t.Fatalf("ListNodes: %v", e)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %+v", nodes)
	}
	n := nodes[0]
	if n.CPUTotal != 3200 {
		t.Fatalf("CPUTotal = %v; want 3200 (static capacity)", n.CPUTotal)
	}
	if n.CPU == 0 {
		t.Fatalf("CPU used = 0; want >0 from stats")
	}
}

func TestWrapMapsNomadErrors(t *testing.T) {
	if e := Wrap(errTest("Unexpected response code: 404")); e.Code != CodeNotFound {
		t.Fatalf("404 -> %s; want not_found", e.Code)
	}
	if e := Wrap(errTest("Unexpected response code: 403 Permission denied")); e.Code != CodeInvalidInput {
		t.Fatalf("403 -> %s; want invalid_input", e.Code)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
