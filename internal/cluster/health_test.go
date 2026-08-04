package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

// newTestMonitor 搭一个最小可用 monitor（pool + cfg + 内存 keyring）。
func newTestMonitor(t *testing.T) (*HealthMonitor, *config.Store, *secure.MemoryKeyring) {
	t.Helper()
	store := config.New(filepath.Join(t.TempDir(), "clusters.json"))
	kr := secure.NewMemory()
	pool := NewPool(store, kr)
	m := NewHealthMonitor(pool, store, 50*time.Millisecond, nil)
	return m, store, kr
}

func TestMonitorEmitsAndCaches(t *testing.T) {
	srv := nomadFake(t, "1.9.4", true)
	m, store, _ := newTestMonitor(t)
	_ = store.Add(&config.ClusterConfig{ID: "c1", Name: "C1", Address: srv.URL})

	var mu sync.Mutex
	got := []HealthUpdate{}
	m.emit = func(u HealthUpdate) {
		mu.Lock()
		got = append(got, u)
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	// 等首轮探测
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("monitor did not emit within 2s")
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	m.Stop()

	mu.Lock()
	defer mu.Unlock()
	if got[0].ClusterID != "c1" || got[0].Status != "ok" {
		t.Fatalf("first emit = %+v", got[0])
	}
	if got[0].Leader == "" {
		t.Fatal("leader empty on ok")
	}

	cached, ok := m.Latest("c1")
	if !ok || cached.Status != "ok" {
		t.Fatalf("Latest = %+v ok=%v", cached, ok)
	}
}

func TestMonitorDetectsDown(t *testing.T) {
	m, store, _ := newTestMonitor(t)
	// 127.0.0.1:1 几乎一定连不上
	_ = store.Add(&config.ClusterConfig{ID: "dead", Name: "D", Address: "http://127.0.0.1:1"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	// 轮询缓存直到出现 down
	deadline := time.After(3 * time.Second)
	for {
		if u, ok := m.Latest("dead"); ok && u.Status == "down" {
			cancel()
			m.Stop()
			if u.Error == "" {
				t.Fatal("down update should carry error message")
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("monitor did not detect down within 3s")
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMonitorInjectUpdatesCache(t *testing.T) {
	m, _, _ := newTestMonitor(t)
	m.Inject(HealthUpdate{ClusterID: "x", Status: "ok", Leader: "l"})
	got, ok := m.Latest("x")
	if !ok || got.Leader != "l" {
		t.Fatalf("Inject did not update cache: %+v ok=%v", got, ok)
	}
}

// nomadFake 起一个最小 nomad API mock。
func nomadFake(t *testing.T, version string, leader bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/status/leader":
			if leader {
				_, _ = w.Write([]byte(`"127.0.0.1:4647"`))
			} else {
				_, _ = w.Write([]byte(`""`))
			}
		case "/v1/agent/self":
			out := map[string]any{
				"config": map[string]any{"version": version},
			}
			b, _ := json.Marshal(out)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
