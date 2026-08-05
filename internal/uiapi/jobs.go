package uiapi

import (
	"strings"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/nomad"
)

// JobsService 承载 jobs 相关的全部 IPC 逻辑（列表 / 详情 / allocations）。
// M1 阶段只读快照 + 手动刷新；M2 的 jobs.patch 事件会复用同一批
// nomad.JobSummary 结构，届时在 store 侧增量合入。
type JobsService struct {
	pool *cluster.Pool
}

// NewJobsService 创建 jobs 服务，复用 ClusterService 的 client 池。
func NewJobsService(pool *cluster.Pool) *JobsService {
	return &JobsService{pool: pool}
}

// ListJobs 返回集群下的全部 job 摘要（首屏全量快照）。
func (s *JobsService) ListJobs(clusterID string) ([]nomad.JobSummary, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	jobs, err := nomad.ListJobs(client)
	if err != nil {
		return nil, Wrap(err)
	}
	return jobs, nil
}

// GetJob 返回单个 job 详情（HCL 定义的 task groups + 运行态汇总）。
func (s *JobsService) GetJob(clusterID, jobID string) (*nomad.JobDetail, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateJobID(jobID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	detail, err := nomad.GetJob(client, jobID)
	if err != nil {
		return nil, Wrap(err)
	}
	return detail, nil
}

// ListJobAllocations 返回 job 下的 allocation 列表。
// allAllocs=false：不含历史 replacement（realloc 会以新 alloc 出现）。
func (s *JobsService) ListJobAllocations(clusterID, jobID string) ([]nomad.AllocSummary, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateJobID(jobID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	allocs, err := nomad.ListJobAllocations(client, jobID)
	if err != nil {
		return nil, Wrap(err)
	}
	return allocs, nil
}

// maxSpecBytes 限制 RunJob 规格大小，防异常大包（1 MiB）。
const maxSpecBytes = 1 << 20

// RunJob 部署（或更新）一个 job：Parse → Validate → Register。
// format ∈ {"hcl", "json"}；namespace 为空时用集群默认 namespace。
// 校验失败返回 CodeInvalidInput，message 携带全部校验错误（前端内联展示）。
func (s *JobsService) RunJob(clusterID, spec, format, namespace string, canonicalize bool) (*nomad.JobRunResult, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	if strings.TrimSpace(spec) == "" {
		return nil, NewError(CodeInvalidInput, "job spec is required")
	}
	if len(spec) > maxSpecBytes {
		return nil, NewError(CodeInvalidInput, "job spec exceeds %d bytes", maxSpecBytes)
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "hcl" && format != "json" {
		return nil, NewError(CodeInvalidInput, "format must be hcl or json")
	}
	if err := ValidateNamespace(namespace); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}

	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	job, err := nomad.ParseJobSpec(client, spec, format, canonicalize)
	if err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	val, err := nomad.ValidateJob(client, job)
	if err != nil {
		return nil, Wrap(err)
	}
	if len(val.ValidationErrors) > 0 {
		return nil, NewError(CodeInvalidInput, "job validation failed:\n%s", strings.Join(val.ValidationErrors, "\n"))
	}
	result, err := nomad.RegisterJob(client, job, namespace)
	if err != nil {
		return nil, Wrap(err)
	}
	return &result, nil
}

// StopJob 停止 job（purge=true 同时清除历史记录），返回 EvalID。
func (s *JobsService) StopJob(clusterID, jobID string, purge bool) (string, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return "", NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateJobID(jobID); err != nil {
		return "", NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return "", Wrap(err)
	}
	evalID, err := nomad.DeregisterJob(client, jobID, purge)
	if err != nil {
		return "", Wrap(err)
	}
	return evalID, nil
}

// EvaluateJob 强制重新评估 job，返回 EvalID。
func (s *JobsService) EvaluateJob(clusterID, jobID string) (string, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return "", NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateJobID(jobID); err != nil {
		return "", NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return "", Wrap(err)
	}
	evalID, err := nomad.ForceEvaluateJob(client, jobID)
	if err != nil {
		return "", Wrap(err)
	}
	return evalID, nil
}

// ScaleJob 对 task group 扩缩容，返回 EvalID。
func (s *JobsService) ScaleJob(clusterID, jobID, group string, count int) (string, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return "", NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateJobID(jobID); err != nil {
		return "", NewError(CodeInvalidInput, "%v", err)
	}
	if strings.TrimSpace(group) == "" {
		return "", NewError(CodeInvalidInput, "task group is required")
	}
	if count < 0 {
		return "", NewError(CodeInvalidInput, "count must be >= 0")
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return "", Wrap(err)
	}
	evalID, err := nomad.ScaleJob(client, jobID, group, count)
	if err != nil {
		return "", Wrap(err)
	}
	return evalID, nil
}

// RestartAlloc 重启 alloc 的任务（taskName 空=全部任务）。
func (s *JobsService) RestartAlloc(clusterID, allocID, taskName string) *Error {
	if err := ValidateClusterID(clusterID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateAllocID(allocID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return Wrap(err)
	}
	if err := nomad.RestartAlloc(client, allocID, taskName); err != nil {
		return Wrap(err)
	}
	return nil
}

// StopAlloc 停止 alloc（触发 reschedule 评估）。
func (s *JobsService) StopAlloc(clusterID, allocID string) *Error {
	if err := ValidateClusterID(clusterID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateAllocID(allocID); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return Wrap(err)
	}
	if err := nomad.StopAlloc(client, allocID); err != nil {
		return Wrap(err)
	}
	return nil
}

// GetEvaluation 返回评估状态（部署进度）。
func (s *JobsService) GetEvaluation(clusterID, evalID string) (*nomad.EvalInfo, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	if strings.TrimSpace(evalID) == "" {
		return nil, NewError(CodeInvalidInput, "eval id is required")
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	info, err := nomad.GetEvaluation(client, evalID)
	if err != nil {
		return nil, Wrap(err)
	}
	return info, nil
}

// ListAllocTaskEvents 返回 alloc 任务事件时间线。
func (s *JobsService) ListAllocTaskEvents(clusterID, allocID string) ([]nomad.AllocTaskEvent, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateAllocID(allocID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	events, err := nomad.ListAllocTaskEvents(client, allocID)
	if err != nil {
		return nil, Wrap(err)
	}
	return events, nil
}

// GetAllocLogs 拉取 alloc 任务日志快照（stdout/stderr）。
func (s *JobsService) GetAllocLogs(clusterID, allocID, task, logType string) (*nomad.AllocLogsResult, *Error) {
	if err := ValidateClusterID(clusterID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateAllocID(allocID); err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	client, err := s.pool.Get(clusterID)
	if err != nil {
		return nil, Wrap(err)
	}
	logs, err := nomad.GetAllocLogs(client, allocID, task, logType)
	if err != nil {
		return nil, Wrap(err)
	}
	return logs, nil
}
