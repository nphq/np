package nomad

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/nomad/api"
)

// logsSDKServer 提供 GetAllocLogs 所需端点：
// alloc info + 404 的 node lookup（强制 SDK 走 server 直连）+ 日志流。
func logsSDKServer(t *testing.T, frames string, handler func(w http.ResponseWriter, r *http.Request)) *api.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/allocation/alloc-1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ID":"alloc-1","JobID":"demo","TaskGroup":"web","ClientStatus":"running","TaskStates":{"echo":{"State":"running","Events":[]}}}`))
	})
	// node lookup 404 → SDK 回退 server 直连（见 fs.go queryClientNode）
	mux.HandleFunc("/v1/node/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/v1/client/fs/logs/alloc-1", func(w http.ResponseWriter, r *http.Request) {
		if handler != nil {
			handler(w, r)
			return
		}
		_, _ = w.Write([]byte(frames))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfg := api.DefaultConfig()
	cfg.Address = srv.URL
	c, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

// TestGetAllocLogs_SDKNormal 走真 SDK（httptest）验证 GetAllocLogs 组装正确。
func TestGetAllocLogs_SDKNormal(t *testing.T) {
	// StreamFrame 的 Data 是 []byte（base64 JSON）；Offset/File 可省。
	frames := `{"Data":"aGVsbG8K"}` + "\n" + `{"Data":"d29ybGQK"}` + "\n"
	client := logsSDKServer(t, frames, nil)

	res, err := GetAllocLogs(context.Background(), client, AllocLogsOpts{
		AllocID: "alloc-1",
		Task:    "echo",
		LogType: "stdout",
	})
	if err != nil {
		t.Fatalf("GetAllocLogs: %v", err)
	}
	if res.Content != "hello\nworld\n" {
		t.Errorf("Content = %q, want hello\\nworld\\n", res.Content)
	}
	if res.AllocID != "alloc-1" || res.Task != "echo" || res.LogType != "stdout" {
		t.Errorf("result meta = %+v", res)
	}
	if res.Truncated {
		t.Error("Truncated = true, want false")
	}
}

// TestGetAllocLogs_SDKTimeout 服务端永不返回时受 opts.Timeout 约束返回截断结果。
func TestGetAllocLogs_SDKTimeout(t *testing.T) {
	client := logsSDKServer(t, "", func(w http.ResponseWriter, r *http.Request) {
		// 挂起连接不写字节。注意：SDK 的 Logs goroutine 阻塞在 dec.Decode，
		// cancel 只在其循环顶部生效，无法打断解码；因此 handler 用短定时
		// 自终止（真正读到数据的路径由 frames EOF 关闭）。
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})

	start := time.Now()
	res, err := GetAllocLogs(context.Background(), client, AllocLogsOpts{
		AllocID: "alloc-1",
		Task:    "echo",
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("GetAllocLogs: %v", err)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true (timeout)")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout took %v, want ~100ms", elapsed)
	}
}

// TestGetAllocLogs_Validation 覆盖入参校验与 task 自动解析。
func TestGetAllocLogs_Validation(t *testing.T) {
	client := logsSDKServer(t, "", nil)
	if _, err := GetAllocLogs(context.Background(), client, AllocLogsOpts{}); err == nil ||
		!strings.Contains(err.Error(), "alloc id is required") {
		t.Errorf("empty alloc id: %v", err)
	}
	// task 空 → 从 TaskStates 自动取第一个
	client2 := logsSDKServer(t, `{"Data":"eA=="}`+"\n", nil)
	res, err := GetAllocLogs(context.Background(), client2, AllocLogsOpts{AllocID: "alloc-1"})
	if err != nil {
		t.Fatalf("GetAllocLogs: %v", err)
	}
	if res.Task != "echo" {
		t.Errorf("auto task = %q, want echo", res.Task)
	}
}

// TestReadLogs_NormalCompletion 覆盖正常流结束：frames 关闭即返回全部内容。
func TestReadLogs_NormalCompletion(t *testing.T) {
	frames := make(chan *api.StreamFrame, 2)
	frames <- &api.StreamFrame{Data: []byte("hello\n")}
	frames <- &api.StreamFrame{Data: []byte("world\n")}
	close(frames)
	errs := make(chan error)

	res, err := readLogs(logsSource{frames: frames, errs: errs}, 64*1024,
		time.After(time.Second), func() {})
	if err != nil {
		t.Fatalf("readLogs: %v", err)
	}
	if res.Content != "hello\nworld\n" {
		t.Errorf("Content = %q, want hello\\nworld\\n", res.Content)
	}
	if res.Truncated {
		t.Error("Truncated = true, want false")
	}
}

// TestReadLogs_Truncation 覆盖超长流：超过 maxBytes 截断并置 Truncated。
func TestReadLogs_Truncation(t *testing.T) {
	frames := make(chan *api.StreamFrame, 1)
	frames <- &api.StreamFrame{Data: make([]byte, 100*1024)} // > 64KiB
	close(frames)
	errs := make(chan error)

	res, err := readLogs(logsSource{frames: frames, errs: errs}, 64*1024,
		time.After(time.Second), func() {})
	if err != nil {
		t.Fatalf("readLogs: %v", err)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(res.Content) != 64*1024 {
		t.Errorf("Content len = %d, want %d", len(res.Content), 64*1024)
	}
}

// TestReadLogs_Timeout 覆盖超时：流不结束且无错误时按截断返回已读内容。
func TestReadLogs_Timeout(t *testing.T) {
	frames := make(chan *api.StreamFrame) // 永不写
	errs := make(chan error)
	timeout := make(chan time.Time, 1)
	timeout <- time.Now()

	res, err := readLogs(logsSource{frames: frames, errs: errs}, 64*1024, timeout, func() {})
	if err != nil {
		t.Fatalf("readLogs: %v", err)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true (timeout)")
	}
	if res.Content != "" {
		t.Errorf("Content = %q, want empty", res.Content)
	}
}

// TestReadLogs_ErrChan 覆盖错误通道：收到非 nil error 立即失败。
func TestReadLogs_ErrChan(t *testing.T) {
	frames := make(chan *api.StreamFrame)
	errs := make(chan error, 1)
	errs <- fmt.Errorf("rpc error")

	_, err := readLogs(logsSource{frames: frames, errs: errs}, 64*1024,
		time.After(time.Second), func() {})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "rpc error") {
		t.Errorf("err = %q, want to contain rpc error", err)
	}
}

// TestReadLogs_ErrsClosedEOF 覆盖 errs 通道关闭（EOF）场景：
// 旧实现会在 errs 关闭后落入 nil-error 分支无限 spin 等 frames；
// 现在应视为流结束立即返回（若非如此，5s 超时会让 Truncated 置 true）。
func TestReadLogs_ErrsClosedEOF(t *testing.T) {
	frames := make(chan *api.StreamFrame)
	close(frames)
	errs := make(chan error)
	close(errs)

	res, err := readLogs(logsSource{frames: frames, errs: errs}, 64*1024,
		time.After(5*time.Second), func() {})
	if err != nil {
		t.Fatalf("readLogs: %v", err)
	}
	if res.Truncated {
		t.Error("Truncated = true, want false (should break on errs close, not timeout)")
	}
	if res.Content != "" {
		t.Errorf("Content = %q, want empty", res.Content)
	}
}

// TestReadLogs_NilErrKeepsReading 覆盖 errs 收到 nil（SDK 同步刷新信号）
// 后流仍继续的场景：nil 不是终止条件。
func TestReadLogs_NilErrKeepsReading(t *testing.T) {
	frames := make(chan *api.StreamFrame, 2)
	frames <- &api.StreamFrame{Data: []byte("before ")}
	errs := make(chan error, 1)
	errs <- nil
	frames <- &api.StreamFrame{Data: []byte("after")}
	close(frames)

	res, err := readLogs(logsSource{frames: frames, errs: errs}, 64*1024,
		time.After(time.Second), func() {})
	if err != nil {
		t.Fatalf("readLogs: %v", err)
	}
	if res.Content != "before after" {
		t.Errorf("Content = %q, want before after", res.Content)
	}
}

// TestReadLogs_FrameThenError 覆盖先收到部分数据再报错的路径。
// frames 不关闭（真实 SDK 在流内错误前不关 frames），保证 select 最终取到 errs。
func TestReadLogs_FrameThenError(t *testing.T) {
	frames := make(chan *api.StreamFrame, 1)
	frames <- &api.StreamFrame{Data: []byte("partial")}
	errs := make(chan error, 1)
	errs <- fmt.Errorf("stream closed by server")

	_, err := readLogs(logsSource{frames: frames, errs: errs}, 64*1024,
		time.After(time.Second), func() {})
	if err == nil || !strings.Contains(err.Error(), "stream closed by server") {
		t.Errorf("err = %v, want stream closed by server", err)
	}
}
