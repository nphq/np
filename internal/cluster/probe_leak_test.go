package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

// TestProbeCtxCancelNoLeak 验证 Probe 在 ctx 取消后不再泄漏 goroutine。
// 旧实现用 goroutine+select 兜底，ctx 取消后内部 goroutine 仍会阻塞在
// SDK Status().Leader() 上直到 http.Client.Timeout(10s) 触发。
// 新实现走 http.NewRequestWithContext，ctx 取消即终止底层 net/http。
func TestProbeCtxCancelNoLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 故意慢：让 ctx 先超时
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`"127.0.0.1:4647"`))
	}))
	defer srv.Close()

	// 强制 GC 让起点稳定
	runtime.GC()
	before := runtime.NumGoroutine()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	u := Probe(ctx, ProbeTarget{Addr: srv.URL, HTTPClient: http.DefaultClient})
	cancel()

	if u.Status != "down" {
		t.Fatalf("want down on ctx cancel, got %+v", u)
	}

	// 给 net/http 时间清理；200ms 远小于旧实现可能 hang 的 10s
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		after := runtime.NumGoroutine()
		if after <= before {
			t.Logf("goroutines before=%d after=%d (returned to baseline)", before, after)
			return
		}
	}

	after := runtime.NumGoroutine()
	if after > before {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}
