package uiapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jobFlowServer 覆盖 RunJob 流水（parse → validate → register）与各管理端点。
func jobFlowServer(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/parse", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		hcl, _ := req["JobHCL"].(string)
		if strings.Contains(hcl, "invalid") {
			_, _ = w.Write([]byte(`{"ID":"invalid","Name":"invalid","TaskGroups":[{"Name":"web","Count":2}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ID":"demo","Name":"demo","Type":"service","TaskGroups":[{"Name":"web","Count":2}]}`))
	})
	mux.HandleFunc("/v1/validate/job", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		id, _ := req["Job"].(map[string]any)["ID"].(string)
		if id == "invalid" {
			_, _ = w.Write([]byte(`{"ValidationErrors":["task web: missing driver"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"DriverConfigValidated":true}`))
	})
	mux.HandleFunc("/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"EvalID":"eval-1","JobModifyIndex":7}`))
	})
	mux.HandleFunc("/v1/job/demo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "want DELETE", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"EvalID":"eval-stop"}`))
	})
	mux.HandleFunc("/v1/job/demo/evaluate", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"EvalID":"eval-force"}`))
	})
	mux.HandleFunc("/v1/job/demo/scale", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"EvalID":"eval-scale"}`))
	})
	mux.HandleFunc("/v1/client/allocation/alloc-1/restart", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/v1/allocation/alloc-1/stop", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"EvalID":"eval-alloc"}`))
	})
	return mux
}

// jobService 构造带 httptest 集群的 JobsService（复用 ClusterService 的池）。
func jobService(t *testing.T, mux *http.ServeMux) *JobsService {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs, _, _ := testService(t)
	if err := cs.AddCluster(ClusterInput{ID: "dev-1", Address: srv.URL}); err != nil {
		t.Fatalf("AddCluster: %v", err)
	}
	return NewJobsService(cs.Pool())
}

func TestRunJob_HCLFlow(t *testing.T) {
	svc := jobService(t, jobFlowServer(t))
	res, e := svc.RunJob(context.Background(), "dev-1", `job "demo" { group "web" { count = 2 } }`, "hcl", "", true)
	if e != nil {
		t.Fatalf("RunJob: %v", e)
	}
	if res.JobID != "demo" || res.EvalID != "eval-1" || res.ModifyIndex != 7 {
		t.Errorf("RunJob result = %+v", res)
	}
}

func TestRunJob_JSONFlow(t *testing.T) {
	svc := jobService(t, jobFlowServer(t))
	res, e := svc.RunJob(context.Background(), "dev-1", `{"ID":"demo","Type":"service"}`, "json", "prod", false)
	if e != nil {
		t.Fatalf("RunJob: %v", e)
	}
	if res.JobID != "demo" {
		t.Errorf("RunJob result = %+v", res)
	}
}

func TestRunJob_ValidationErrors(t *testing.T) {
	svc := jobService(t, jobFlowServer(t))
	_, e := svc.RunJob(context.Background(), "dev-1", `job "invalid" { group "web" { count = 2 } }`, "hcl", "", true)
	if e == nil || e.Code != CodeInvalidInput {
		t.Fatalf("want CodeInvalidInput, got %v", e)
	}
	if !strings.Contains(e.Message, "missing driver") {
		t.Errorf("message should carry validation errors: %q", e.Message)
	}
}

func TestRunJob_InputValidation(t *testing.T) {
	svc := jobService(t, jobFlowServer(t))
	cases := []struct {
		spec, format, ns string
	}{
		{"", "hcl", ""},                                  // 空规格
		{"x", "yaml", ""},                                // 未知格式
		{`{"ID":"a"}`, "json", "a b"},                    // 非法 namespace
		{strings.Repeat("x", maxSpecBytes+1), "hcl", ""}, // 超长规格
	}
	for i, c := range cases {
		if _, e := svc.RunJob(context.Background(), "dev-1", c.spec, c.format, c.ns, false); e == nil || e.Code != CodeInvalidInput {
			t.Errorf("case %d: want invalid_input, got %v", i, e)
		}
	}
	if _, e := svc.RunJob(context.Background(), "nope", "x", "hcl", "", false); e == nil {
		t.Error("bad clusterID should error")
	}
}

func TestStopEvaluateScale(t *testing.T) {
	svc := jobService(t, jobFlowServer(t))
	if id, e := svc.StopJob(context.Background(), "dev-1", "demo", false); e != nil || id != "eval-stop" {
		t.Errorf("StopJob = %q, %v", id, e)
	}
	if id, e := svc.EvaluateJob(context.Background(), "dev-1", "demo"); e != nil || id != "eval-force" {
		t.Errorf("EvaluateJob = %q, %v", id, e)
	}
	if id, e := svc.ScaleJob(context.Background(), "dev-1", "demo", "web", 5); e != nil || id != "eval-scale" {
		t.Errorf("ScaleJob = %q, %v", id, e)
	}
	// 非法入参
	if _, e := svc.StopJob(context.Background(), "dev-1", "bad id!", false); e == nil {
		t.Error("bad jobID should error")
	}
	if _, e := svc.ScaleJob(context.Background(), "dev-1", "demo", "", 1); e == nil {
		t.Error("empty group should error")
	}
	if _, e := svc.ScaleJob(context.Background(), "dev-1", "demo", "web", -1); e == nil {
		t.Error("negative count should error")
	}
}

func TestRestartStopAlloc(t *testing.T) {
	svc := jobService(t, jobFlowServer(t))
	if e := svc.RestartAlloc(context.Background(), "dev-1", "alloc-1", "web"); e != nil {
		t.Errorf("RestartAlloc: %v", e)
	}
	if e := svc.RestartAlloc(context.Background(), "dev-1", "alloc-1", ""); e != nil {
		t.Errorf("RestartAlloc all tasks: %v", e)
	}
	if e := svc.StopAlloc(context.Background(), "dev-1", "alloc-1"); e != nil {
		t.Errorf("StopAlloc: %v", e)
	}
	if e := svc.RestartAlloc(context.Background(), "dev-1", "bad alloc!", ""); e == nil {
		t.Error("bad allocID should error")
	}
}
