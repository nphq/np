package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hashicorp/nomad/api"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

func testPool(t *testing.T) (*Pool, *config.Store, *secure.MemoryKeyring) {
	t.Helper()
	dir := t.TempDir()
	store := config.New(filepath.Join(dir, "clusters.json"))
	kr := secure.NewMemory()
	p := NewPool(store, kr)
	return p, store, kr
}

func TestPoolGetCreatesOnce(t *testing.T) {
	p, store, kr := testPool(t)
	if err := store.Add(&config.ClusterConfig{
		ID: "c1", Name: "C1", Address: "http://127.0.0.1:4646",
	}); err != nil {
		t.Fatal(err)
	}
	_ = kr.SaveToken("c1", "tok-1")

	var mu sync.Mutex
	calls := 0
	p.factory = func(cfg *config.ClusterConfig, token string) (*api.Client, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if cfg.Address != "http://127.0.0.1:4646" {
			t.Fatalf("unexpected address %q", cfg.Address)
		}
		if token != "tok-1" {
			t.Fatalf("unexpected token %q", token)
		}
		return api.NewClient(api.DefaultConfig())
	}

	// 并发取同一集群，factory 只应被调一次
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Get("c1"); err != nil {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("factory calls = %d, want 1", calls)
	}
}

func TestPoolGetUnknownCluster(t *testing.T) {
	p, _, _ := testPool(t)
	if _, err := p.Get("nope"); err == nil {
		t.Fatal("want error for unknown cluster")
	}
}

func TestPoolGetWithoutToken(t *testing.T) {
	p, store, _ := testPool(t)
	if err := store.Add(&config.ClusterConfig{
		ID: "c1", Name: "C1", Address: "http://127.0.0.1:4646",
	}); err != nil {
		t.Fatal(err)
	}
	// 不存 token：无 token 也应能建 client（连接阶段才可能 401）
	if _, err := p.Get("c1"); err != nil {
		t.Fatalf("Get without token: %v", err)
	}
}

func TestFactorySchemeDefault(t *testing.T) {
	// 无 scheme 时默认补 http://
	cfg := &config.ClusterConfig{ID: "c1", Address: "127.0.0.1:4646"}
	client, err := DefaultClientFactory(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if client.Address() != "http://127.0.0.1:4646" {
		t.Fatalf("address = %q", client.Address())
	}
}

func TestProbeAgainstRealServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status/leader" {
			_, _ = w.Write([]byte("\"127.0.0.1:4647\""))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u := Probe(context.Background(), ProbeTarget{Addr: srv.URL, HTTPClient: http.DefaultClient})
	if u.Status != "ok" {
		t.Fatalf("Probe: status=%s err=%s", u.Status, u.Error)
	}
	if u.Leader != "127.0.0.1:4647" {
		t.Fatalf("Probe: leader=%q", u.Leader)
	}
}

func TestProbeNoLeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status/leader" {
			_, _ = w.Write([]byte(`""`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u := Probe(context.Background(), ProbeTarget{Addr: srv.URL, HTTPClient: http.DefaultClient})
	if u.Status != "down" {
		t.Fatalf("want down, got %+v", u)
	}
}

func TestPoolClose(t *testing.T) {
	p, store, kr := testPool(t)
	_ = store.Add(&config.ClusterConfig{ID: "c1", Name: "C1", Address: "http://127.0.0.1:4646"})
	_ = kr.SaveToken("c1", "t")
	p.factory = func(cfg *config.ClusterConfig, token string) (*api.Client, error) {
		return api.NewClient(api.DefaultConfig())
	}
	if _, err := p.Get("c1"); err != nil {
		t.Fatal(err)
	}
	p.Close()
	// 关闭后仍可重建
	if _, err := p.Get("c1"); err != nil {
		t.Fatalf("Get after Close: %v", err)
	}
}

// TestPoolInvalidate 验证删除单集群后，下次 Get 用新 token 重建。
// 防 Bug 2：RemoveCluster 后用同 ID 重加，pool 仍返回带旧 token 的旧 client。
func TestPoolInvalidate(t *testing.T) {
	p, store, kr := testPool(t)
	_ = store.Add(&config.ClusterConfig{ID: "c1", Name: "C1", Address: "http://127.0.0.1:4646"})
	_ = kr.SaveToken("c1", "old-token")

	var mu sync.Mutex
	var seen []string
	p.factory = func(cfg *config.ClusterConfig, token string) (*api.Client, error) {
		mu.Lock()
		seen = append(seen, token)
		mu.Unlock()
		return api.NewClient(api.DefaultConfig())
	}

	// 第一次 Get：factory 看到 old-token
	if _, err := p.Get("c1"); err != nil {
		t.Fatal(err)
	}
	// 模拟"删 → 改 token → 重加同 ID"
	p.Invalidate("c1")
	_ = kr.SaveToken("c1", "new-token")
	if _, err := p.Get("c1"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != "old-token" || seen[1] != "new-token" {
		t.Fatalf("factory token sequence = %v, want [old-token new-token]", seen)
	}
}

// TestPoolInvalidateUnknown 安全：invalidate 不存在的集群不 panic。
func TestPoolInvalidateUnknown(t *testing.T) {
	p, _, _ := testPool(t)
	p.Invalidate("never-existed") // 不应 panic
}
