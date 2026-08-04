package uiapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

func TestLoadsServiceValidation(t *testing.T) {
	svc := NewLoadsService(cluster.NewPool(config.New(t.TempDir()+"/c.json"), secure.NewMemory()))
	if _, e := svc.GetClusterLoad("bad id!"); e == nil || e.Code != CodeInvalidInput {
		t.Fatalf("want invalid_input for bad cluster id, got %+v", e)
	}
	if _, e := svc.GetAllocLoad("ok", "bad id!"); e == nil || e.Code != CodeInvalidInput {
		t.Fatalf("want invalid_input for bad alloc id, got %+v", e)
	}
}

func TestLoadsServiceGetClusterLoad_SyncFirstPaint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes":
			_, _ = w.Write([]byte(`[{"ID":"n1","Name":"node-1","Status":"ready","NodeResources":{"Cpu":{"CpuShares":3200,"TotalCpuCores":4},"Memory":{"MemoryMB":8192},"Disk":{"DiskMB":100000}}}]`))
		case "/v1/allocations":
			_, _ = w.Write([]byte(`[]`))
		case "/v1/client/stats":
			_, _ = w.Write([]byte(`{"Memory":{"Total":8589934592,"Used":1073741824},"CPU":[{"CPU":"cpu0","Idle":50}],"DiskStats":[{"Used":52428800}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	store := config.New(t.TempDir() + "/clusters.json")
	if err := store.Add(&config.ClusterConfig{ID: "dev", Name: "Dev", Address: srv.URL}); err != nil {
		t.Fatal(err)
	}
	pool := cluster.NewPool(store, secure.NewMemory())
	svc := NewLoadsService(pool)

	cl, e := svc.GetClusterLoad("dev")
	if e != nil {
		t.Fatalf("GetClusterLoad: %v", e)
	}
	if cl.NodeCount != 1 || cl.NodeUp != 1 {
		t.Fatalf("cluster = %+v", cl)
	}
	if cl.Capacity.CPU != 3200 || cl.Used.CPU != 1600 {
		t.Fatalf("capacity=%v used=%v", cl.Capacity.CPU, cl.Used.CPU)
	}
	if cl.Samples == nil || len(cl.Samples) != 1 {
		t.Fatalf("samples = %+v", cl.Samples)
	}

	// 二次调用走缓存（同步 tick 不再触发网络）
	before := len(cl.Samples)
	cl2, e := svc.GetClusterLoad("dev")
	if e != nil {
		t.Fatal(e)
	}
	if len(cl2.Samples) != before {
		t.Fatalf("second call should hit cache, samples %d → %d", before, len(cl2.Samples))
	}
}
