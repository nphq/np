package nomad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/nomad/api"
)

// 指针字面量助手，避免每个用例都写 helper。
func sp(s string) *string { return &s }
func ip(i int) *int       { return &i }
func up(u uint64) *uint64 { return &u }

// sdkClient 基于 httptest 构造 api.Client（Address 指向测试桩）。
func sdkClient(t *testing.T, h http.HandlerFunc) *api.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cfg := api.DefaultConfig()
	cfg.Address = srv.URL
	c, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestMapJobListStub_AggregatesAcrossTaskGroups(t *testing.T) {
	stub := &api.JobListStub{
		ID:       "redis",
		Name:     "redis",
		Type:     "service",
		Priority: 50,
		Status:   "running",
		JobSummary: &api.JobSummary{
			Summary: map[string]api.TaskGroupSummary{
				"web": {
					Running: 3, Queued: 1, Starting: 2, Failed: 0,
					Complete: 0, Lost: 0, Unknown: 1,
				},
				"worker": {
					Running: 5, Queued: 0, Starting: 0, Failed: 2,
					Complete: 4, Lost: 1, Unknown: 0,
				},
			},
		},
	}

	got := mapJobListStub(stub)

	if got.ID != "redis" {
		t.Errorf("ID = %q, want redis", got.ID)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.Running != 8 {
		t.Errorf("Running = %d, want 8 (3+5 across groups)", got.Running)
	}
	if got.Pending != 3 {
		t.Errorf("Pending = %d, want 3 (starting+unknown: 2+1)", got.Pending)
	}
	if got.Dead != 5 {
		t.Errorf("Dead = %d, want 5 (complete+lost: 4+1)", got.Dead)
	}
}

func TestMapJobListStub_NilJobSummary(t *testing.T) {
	// periodic job 在某些版本下 JobSummary 可能为 nil；映射必须不 panic。
	stub := &api.JobListStub{
		ID:       "periodic-job",
		Name:     "periodic-job",
		Type:     "batch",
		Priority: 10,
		Status:   "running",
	}
	got := mapJobListStub(stub)
	if got.ID != "periodic-job" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Running != 0 || got.Pending != 0 {
		t.Errorf("nil JobSummary: Running=%d Pending=%d, want zeros", got.Running, got.Pending)
	}
}

func TestMapJobDetail_PointerDerefAndTaskGroups(t *testing.T) {
	job := &api.Job{
		ID:          sp("redis"),
		Name:        sp("redis-cache"),
		Namespace:   sp("default"),
		Type:        sp("service"),
		Status:      sp("running"),
		Priority:    ip(50),
		Datacenters: []string{"dc1", "dc2"},
		CreateIndex: up(100),
		ModifyIndex: up(150),
		TaskGroups: []*api.TaskGroup{
			{Name: sp("web"), Count: ip(3)},
			{Name: sp("worker"), Count: ip(2)},
			// nil entry 必须被跳过而非 panic
			nil,
		},
	}
	summary := map[string]api.TaskGroupSummary{
		"web":    {Running: 3, Queued: 0, Starting: 1, Failed: 0, Complete: 0, Lost: 0, Unknown: 0},
		"worker": {Running: 2, Queued: 1, Starting: 0, Failed: 1, Complete: 0, Lost: 0, Unknown: 0},
	}

	d := mapJobDetail(job, summary)

	if d.ID != "redis" || d.Name != "redis-cache" || d.Namespace != "default" {
		t.Errorf("basic fields: %+v", d)
	}
	if d.Priority != 50 || d.Status != "running" || d.Type != "service" {
		t.Errorf("enum fields: %+v", d)
	}
	if len(d.Datacenters) != 2 || d.Datacenters[0] != "dc1" {
		t.Errorf("Datacenters = %v", d.Datacenters)
	}
	if d.CreateIndex != 100 || d.ModifyIndex != 150 {
		t.Errorf("indexes: create=%d modify=%d", d.CreateIndex, d.ModifyIndex)
	}

	if len(d.TaskGroups) != 2 {
		t.Fatalf("TaskGroups len = %d, want 2 (nil entry skipped)", len(d.TaskGroups))
	}
	web := d.TaskGroups[0]
	if web.Name != "web" || web.Count != 3 || web.Running != 3 || web.Pending != 1 {
		t.Errorf("web group: %+v", web)
	}
	worker := d.TaskGroups[1]
	if worker.Name != "worker" || worker.Count != 2 || worker.Running != 2 || worker.Queued != 1 || worker.Failed != 1 {
		t.Errorf("worker group: %+v", worker)
	}

	// 顶部 Summary 应聚合两个组
	if d.Summary.Running != 5 || d.Summary.Queued != 1 || d.Summary.Pending != 1 || d.Summary.Failed != 1 {
		t.Errorf("Summary aggregate: %+v", d.Summary)
	}
}

func TestMapJobDetail_NilPointerFieldsSafe(t *testing.T) {
	// Info 端点理论上不会返回 nil 字段，但防御：所有指针字段都 nil 也不应 panic。
	job := &api.Job{
		TaskGroups: []*api.TaskGroup{
			{}, // Name/Count 都 nil
		},
	}
	d := mapJobDetail(job, nil)
	if d.ID != "" || d.Priority != 0 {
		t.Errorf("nil pointers should map to zero: %+v", d)
	}
	if len(d.TaskGroups) != 1 || d.TaskGroups[0].Name != "" || d.TaskGroups[0].Count != 0 {
		t.Errorf("tg with nil fields: %+v", d.TaskGroups)
	}
}

func TestStrIntUintDeref_NilSafe(t *testing.T) {
	if strDeref(nil) != "" {
		t.Error("strDeref(nil) should be empty")
	}
	if strDeref(sp("x")) != "x" {
		t.Error("strDeref(non-nil) broken")
	}
	if intDeref(nil) != 0 || intDeref(ip(7)) != 7 {
		t.Error("intDeref broken")
	}
	if uint64Deref(nil) != 0 || uint64Deref(up(42)) != 42 {
		t.Error("uint64Deref broken")
	}
}

func TestParseJobSpec_HCL(t *testing.T) {
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs/parse" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req["JobHCL"] != "job \"demo\" {}" {
			http.Error(w, "wrong JobHCL", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"ID":"demo","Name":"demo","Type":"service","TaskGroups":[{"Name":"web","Count":2}]}`))
	})
	job, err := ParseJobSpec(client, "job \"demo\" {}", "hcl", true)
	if err != nil {
		t.Fatalf("ParseJobSpec: %v", err)
	}
	if strDeref(job.ID) != "demo" || job.TaskGroups[0].Name == nil || strDeref(job.TaskGroups[0].Name) != "web" {
		t.Errorf("parsed job mismatch: %+v", job)
	}
	if intDeref(job.TaskGroups[0].Count) != 2 {
		t.Errorf("Count = %d, want 2", intDeref(job.TaskGroups[0].Count))
	}
}

func TestParseJobSpec_JSON(t *testing.T) {
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no server call expected", http.StatusInternalServerError)
	})
	spec := `{"ID":"demo","Type":"batch","TaskGroups":[{"Name":"web","Count":3}]}`
	job, err := ParseJobSpec(client, spec, "json", false)
	if err != nil {
		t.Fatalf("ParseJobSpec json: %v", err)
	}
	if strDeref(job.ID) != "demo" || strDeref(job.Type) != "batch" || intDeref(job.TaskGroups[0].Count) != 3 {
		t.Errorf("json parse mismatch: %+v", job)
	}
}

func TestParseJobSpec_JSONMalformed(t *testing.T) {
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no server call expected", http.StatusInternalServerError)
	})
	if _, err := ParseJobSpec(client, `{"ID":`, "json", false); err == nil {
		t.Error("want error for malformed JSON")
	}
}

func TestParseJobSpec_BadFormat(t *testing.T) {
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no server call expected", http.StatusInternalServerError)
	})
	if _, err := ParseJobSpec(client, "x", "yaml", false); err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("want unsupported format error, got %v", err)
	}
}

func TestValidateJob(t *testing.T) {
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/validate/job" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"DriverConfigValidated":true,"ValidationErrors":["1 error occurred: task web: missing driver"],"Warnings":"deprecated field"}`))
	})
	job := &api.Job{ID: sp("demo")}
	res, err := ValidateJob(context.Background(), client, job)
	if err != nil {
		t.Fatalf("ValidateJob: %v", err)
	}
	if !res.DriverConfigValidated || len(res.ValidationErrors) != 1 || !strings.Contains(res.ValidationErrors[0], "missing driver") {
		t.Errorf("validate result mismatch: %+v", res)
	}
	if res.Warnings != "deprecated field" {
		t.Errorf("warnings mismatch: %+v", res)
	}
}

func TestRegisterJob(t *testing.T) {
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"EvalID":"e1","JobModifyIndex":42,"Warnings":"none"}`))
	})
	job := &api.Job{ID: sp("demo")}
	res, err := RegisterJob(context.Background(), client, job, "")
	if err != nil {
		t.Fatalf("RegisterJob: %v", err)
	}
	if res.JobID != "demo" || res.EvalID != "e1" || res.ModifyIndex != 42 {
		t.Errorf("register result mismatch: %+v", res)
	}
}

func TestDeregisterJob(t *testing.T) {
	var purgeQuery string
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/demo" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		purgeQuery = r.URL.Query().Get("purge")
		_, _ = w.Write([]byte(`{"EvalID":"e2"}`))
	})
	evalID, err := DeregisterJob(context.Background(), client, "demo", true, "")
	if err != nil {
		t.Fatalf("DeregisterJob: %v", err)
	}
	if evalID != "e2" || purgeQuery != "true" {
		t.Errorf("deregister: evalID=%q purge=%q", evalID, purgeQuery)
	}
}

func TestForceEvaluateJob(t *testing.T) {
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/demo/evaluate" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"EvalID":"e3"}`))
	})
	evalID, err := ForceEvaluateJob(context.Background(), client, "demo", "")
	if err != nil {
		t.Fatalf("ForceEvaluateJob: %v", err)
	}
	if evalID != "e3" {
		t.Errorf("evalID = %q, want e3", evalID)
	}
}

func TestScaleJob(t *testing.T) {
	var gotCount int64
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/demo/scale" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		var req struct {
			Count  *int64            `json:"Count"`
			Target map[string]string `json:"Target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Count != nil {
			gotCount = *req.Count
		}
		if req.Target["Group"] != "web" {
			http.Error(w, "wrong target group", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"EvalID":"e4"}`))
	})
	evalID, err := ScaleJob(context.Background(), client, "demo", "web", 5, "")
	if err != nil {
		t.Fatalf("ScaleJob: %v", err)
	}
	if evalID != "e4" || gotCount != 5 {
		t.Errorf("scale: evalID=%q count=%d", evalID, gotCount)
	}
}

func TestRestartAlloc(t *testing.T) {
	var path, task string
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		var req struct {
			TaskName string `json:"TaskName"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		task = req.TaskName
		_, _ = w.Write([]byte(`{}`))
	})
	if err := RestartAlloc(context.Background(), client, "alloc-1", "web"); err != nil {
		t.Fatalf("RestartAlloc: %v", err)
	}
	if path != "/v1/client/allocation/alloc-1/restart" || task != "web" {
		t.Errorf("restart: path=%q task=%q", path, task)
	}
}

func TestStopAlloc(t *testing.T) {
	var path string
	client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"EvalID":"e5"}`))
	})
	if err := StopAlloc(context.Background(), client, "alloc-1"); err != nil {
		t.Fatalf("StopAlloc: %v", err)
	}
	if path != "/v1/allocation/alloc-1/stop" {
		t.Errorf("stop path = %q", path)
	}
}

// TestListJobs_NamespacePropagates 验证集群 namespace 传导到请求参数：
// 回归（review P0）——之前 pool 的 acfg.Namespace 不生效，ListJobs 永远查 default。
// 空 namespace 时请求不带参数（服务端回退 default，行为同旧版）。
func TestListJobs_NamespacePropagates(t *testing.T) {
	t.Run("explicit namespace", func(t *testing.T) {
		var gotNS string
		client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotNS = r.URL.Query().Get("namespace")
			_, _ = w.Write([]byte(`[]`))
		})
		if _, err := ListJobs(context.Background(), client, "dev"); err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if gotNS != "dev" {
			t.Fatalf("namespace param = %q, want dev", gotNS)
		}
	})
	t.Run("empty namespace omits param", func(t *testing.T) {
		var gotRawQuery string
		client := sdkClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotRawQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`[]`))
		})
		if _, err := ListJobs(context.Background(), client, ""); err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if strings.Contains(gotRawQuery, "namespace") {
			t.Fatalf("unexpected namespace param in %q", gotRawQuery)
		}
	})
}
