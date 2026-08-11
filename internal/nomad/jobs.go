package nomad

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// 本文件是 nomad/api SDK → DTO 的纯函数映射层，无状态、可单测。
// 隔离 SDK 类型漂移（report §18 ADR-2）：所有 SDK struct 字段都是指针，
// 这里集中处理 deref；上层只消费稳定的 DTO。

// strDeref 安全解引用 *string。
func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// intDeref 安全解引用 *int。
func intDeref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// uint64Deref 安全解引用 *uint64。
func uint64Deref(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

// ListJobs 拉取 job 列表并映射为 []JobSummary。
// 每个 stub 的运行态计数来自 JobSummary.Summary（按 task group 聚合）；
// nil JobSummary（periodic / parameterized 罕见情况）返回零计数。
func ListJobs(ctx context.Context, client *api.Client) ([]JobSummary, error) {
	stubs, _, err := client.Jobs().List((&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	out := make([]JobSummary, 0, len(stubs))
	for _, s := range stubs {
		out = append(out, mapJobListStub(s))
	}
	return out, nil
}

func mapJobListStub(s *api.JobListStub) JobSummary {
	js := JobSummary{
		ID:       s.ID,
		Name:     s.Name,
		Type:     s.Type,
		Priority: s.Priority,
		Status:   s.Status,
	}
	if s.JobSummary != nil {
		for _, tgs := range s.JobSummary.Summary {
			js.Running += tgs.Running
			js.Queued += tgs.Queued
			js.Pending += tgs.Starting + tgs.Unknown
			js.Failed += tgs.Failed
			js.Dead += tgs.Lost + tgs.Complete
		}
	}
	return js
}

// GetJob 拉取单个 job 详情，包含 HCL 定义的 task groups 与对应运行态汇总。
// 内部两次 API：Info 拿 job 定义 + task group 期望 count，Summary 拿运行态计数。
// Summary 失败不致命：返回 detail 但 TaskGroups 各状态字段为 0。
func GetJob(ctx context.Context, client *api.Client, jobID string) (*JobDetail, error) {
	job, _, err := client.Jobs().Info(jobID, (&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", jobID, err)
	}
	summary := map[string]api.TaskGroupSummary{}
	if js, _, sErr := client.Jobs().Summary(jobID, (&api.QueryOptions{}).WithContext(ctx)); sErr == nil && js != nil {
		summary = js.Summary
	}
	return mapJobDetail(job, summary), nil
}

// mapJobDetail 是 Info + Summary → JobDetail 的纯映射。
// 抽出来便于单测：不用真起 Nomad 也能验字段对齐 / nil 指针安全。
func mapJobDetail(job *api.Job, summary map[string]api.TaskGroupSummary) *JobDetail {
	d := &JobDetail{
		ID:          strDeref(job.ID),
		Name:        strDeref(job.Name),
		Namespace:   strDeref(job.Namespace),
		Type:        strDeref(job.Type),
		Status:      strDeref(job.Status),
		Priority:    intDeref(job.Priority),
		Datacenters: job.Datacenters,
		CreateIndex: uint64Deref(job.CreateIndex),
		ModifyIndex: uint64Deref(job.ModifyIndex),
	}
	d.Summary = JobSummary{
		ID:       d.ID,
		Name:     d.Name,
		Type:     d.Type,
		Priority: d.Priority,
		Status:   d.Status,
	}

	d.TaskGroups = make([]TaskGroupInfo, 0, len(job.TaskGroups))
	for _, tg := range job.TaskGroups {
		if tg == nil {
			continue
		}
		name := strDeref(tg.Name)
		tgs := summary[name]
		d.TaskGroups = append(d.TaskGroups, TaskGroupInfo{
			Name:     name,
			Count:    intDeref(tg.Count),
			Running:  tgs.Running,
			Queued:   tgs.Queued,
			Pending:  tgs.Starting + tgs.Unknown,
			Failed:   tgs.Failed,
			Complete: tgs.Complete,
			Lost:     tgs.Lost,
		})
		d.Summary.Running += tgs.Running
		d.Summary.Queued += tgs.Queued
		d.Summary.Pending += tgs.Starting + tgs.Unknown
		d.Summary.Failed += tgs.Failed
		d.Summary.Dead += tgs.Complete + tgs.Lost
	}
	return d
}

// ListJobAllocations 拉取 job 下的分配列表。
// allAllocs=false：只返回最近一次 evaluation 的 allocs（不含历史 replacement）。
func ListJobAllocations(ctx context.Context, client *api.Client, jobID string) ([]AllocSummary, error) {
	allocs, _, err := client.Jobs().Allocations(jobID, false, (&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list allocations for job %s: %w", jobID, err)
	}
	out := make([]AllocSummary, 0, len(allocs))
	for _, a := range allocs {
		out = append(out, AllocSummary{
			ID:            a.ID,
			JobID:         a.JobID,
			TaskGroup:     a.TaskGroup,
			NodeID:        a.NodeID,
			NodeName:      a.NodeName,
			Status:        a.ClientStatus,
			ClientStatus:  a.ClientStatus,
			DesiredStatus: a.DesiredStatus,
			EvalID:        a.EvalID,
			CreateIndex:   a.CreateIndex,
			ModifyIndex:   a.ModifyIndex,
		})
	}
	return out, nil
}

// ParseJobSpec 把用户提交的规格文本解析为 api.Job。
// format ∈ {"hcl", "json"}：hcl 走服务端 ParseHCL（含 canonicalize）；
// json 走本地 json.Unmarshal（字段名与 SDK Go 字段对齐，即 /v1/jobs/parse 输出）。
//
// 注：SDK 的 ParseHCL/ParseHCLOpts 不接收 Query/WriteOptions（ctx 无法下沉），
// 本函数刻意不带 ctx 参数；其它走 options 的调用全部透传 ctx。
func ParseJobSpec(client *api.Client, spec, format string, canonicalize bool) (*api.Job, error) {
	switch strings.ToLower(format) {
	case "hcl":
		job, err := client.Jobs().ParseHCL(spec, canonicalize)
		if err != nil {
			return nil, fmt.Errorf("parse HCL: %w", err)
		}
		if job == nil {
			return nil, fmt.Errorf("parse HCL: empty job")
		}
		return job, nil
	case "json":
		var job api.Job
		if err := json.Unmarshal([]byte(spec), &job); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
		return &job, nil
	default:
		return nil, fmt.Errorf("unsupported format %q (want hcl|json)", format)
	}
}

// ValidateJob 请求服务端校验 job，返回校验结果 DTO。
func ValidateJob(ctx context.Context, client *api.Client, job *api.Job) (JobValidateResult, error) {
	resp, _, err := client.Jobs().Validate(job, (&api.WriteOptions{}).WithContext(ctx))
	if err != nil {
		return JobValidateResult{}, fmt.Errorf("validate job: %w", err)
	}
	return JobValidateResult{
		DriverConfigValidated: resp.DriverConfigValidated,
		ValidationErrors:      resp.ValidationErrors,
		Warnings:              resp.Warnings,
	}, nil
}

// RegisterJob 提交/更新 job（PUT /v1/jobs），返回回执。
// namespace 非空时覆盖集群默认 namespace。
func RegisterJob(ctx context.Context, client *api.Client, job *api.Job, namespace string) (JobRunResult, error) {
	var wq *api.WriteOptions
	if namespace != "" {
		wq = (&api.WriteOptions{Namespace: namespace}).WithContext(ctx)
	} else {
		wq = (&api.WriteOptions{}).WithContext(ctx)
	}
	resp, _, err := client.Jobs().Register(job, wq)
	if err != nil {
		return JobRunResult{}, fmt.Errorf("register job: %w", err)
	}
	return JobRunResult{
		JobID:       strDeref(job.ID),
		EvalID:      resp.EvalID,
		Warnings:    resp.Warnings,
		ModifyIndex: resp.JobModifyIndex,
	}, nil
}

// DeregisterJob 停止 job（purge=true 同时清除历史记录），返回 EvalID。
func DeregisterJob(ctx context.Context, client *api.Client, jobID string, purge bool) (string, error) {
	evalID, _, err := client.Jobs().Deregister(jobID, purge, (&api.WriteOptions{}).WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("deregister job %s: %w", jobID, err)
	}
	return evalID, nil
}

// ForceEvaluateJob 强制重新评估 job，返回 EvalID。
func ForceEvaluateJob(ctx context.Context, client *api.Client, jobID string) (string, error) {
	evalID, _, err := client.Jobs().ForceEvaluate(jobID, (&api.WriteOptions{}).WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("force evaluate job %s: %w", jobID, err)
	}
	return evalID, nil
}

// ScaleJob 对 task group 扩缩容（PUT /v1/job/{id}/scale），返回 EvalID。
func ScaleJob(ctx context.Context, client *api.Client, jobID, group string, count int) (string, error) {
	resp, _, err := client.Jobs().Scale(jobID, group, &count, "scaled from Nomad Panel", false, nil, (&api.WriteOptions{}).WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("scale job %s group %s: %w", jobID, group, err)
	}
	return resp.EvalID, nil
}

// RestartAlloc 重启 alloc 的全部任务或指定任务（taskName 为空=全部）。
func RestartAlloc(ctx context.Context, client *api.Client, allocID, taskName string) error {
	alloc := &api.Allocation{ID: allocID}
	if err := client.Allocations().Restart(alloc, taskName, (&api.QueryOptions{}).WithContext(ctx)); err != nil {
		return fmt.Errorf("restart alloc %s: %w", allocID, err)
	}
	return nil
}

// StopAlloc 停止 alloc（触发 reschedule 评估），无 EvalID 返回。
func StopAlloc(ctx context.Context, client *api.Client, allocID string) error {
	alloc := &api.Allocation{ID: allocID}
	if _, err := client.Allocations().Stop(alloc, (&api.QueryOptions{}).WithContext(ctx)); err != nil {
		return fmt.Errorf("stop alloc %s: %w", allocID, err)
	}
	return nil
}
