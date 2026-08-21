package nomad

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/nomad/api"
)

const (
	// defaultLogTimeout 是 GetAllocLogs 的默认流超时（uiapi 层可覆盖）。
	defaultLogTimeout = 8 * time.Second
	// defaultLogMaxBytes 是日志快照的默认截断上限。
	defaultLogMaxBytes = 64 * 1024
)

// GetEvaluation 查询评估状态（部署进度 / blocked 原因）。
func GetEvaluation(ctx context.Context, client *api.Client, evalID string) (*EvalInfo, error) {
	if strings.TrimSpace(evalID) == "" {
		return nil, fmt.Errorf("eval id is required")
	}
	ev, _, err := client.Evaluations().Info(evalID, (&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get evaluation %s: %w", evalID, err)
	}
	if ev == nil {
		return nil, fmt.Errorf("evaluation %s not found", evalID)
	}
	out := &EvalInfo{
		ID:                ev.ID,
		JobID:             ev.JobID,
		Status:            ev.Status,
		StatusDescription: ev.StatusDescription,
		Type:              ev.Type,
		Priority:          ev.Priority,
		BlockedEval:       ev.BlockedEval,
	}
	if !ev.WaitUntil.IsZero() {
		out.WaitUntil = ev.WaitUntil.UnixMilli()
	}
	if len(ev.FailedTGAllocs) > 0 {
		out.FailedSummary = evalFailedSummary(ev.FailedTGAllocs)
	}
	return out, nil
}

// evalFailedSummary 把调度失败指标转成人类可读摘要。
// 每个 task group 优先展示节点被过滤的具体原因（ConstraintFiltered / ClassFiltered，
// 如 "missing drivers"），再附计数；无原因时回退到原有计数描述。
// 提取为纯函数便于单测（不依赖 SDK 调用）。
func evalFailedSummary(failed map[string]*api.AllocationMetric) string {
	groups := make([]string, 0, len(failed))
	for tg, metric := range failed {
		groups = append(groups, failedGroupSummary(tg, metric))
	}
	sort.Strings(groups)
	return strings.Join(groups, "; ")
}

func failedGroupSummary(tg string, metric *api.AllocationMetric) string {
	if metric == nil {
		return tg + ": placement failed"
	}
	if reasons := filterReasons(metric); len(reasons) > 0 {
		return fmt.Sprintf("%s: %s (coalescedFailures=%d nodesEvaluated=%d nodesFiltered=%d)",
			tg, strings.Join(reasons, "; "), metric.CoalescedFailures, metric.NodesEvaluated, metric.NodesFiltered)
	}
	avail := 0
	for _, n := range metric.NodesAvailable {
		avail += n
	}
	if metric.CoalescedFailures == 0 && avail == 0 && metric.NodesEvaluated > 0 {
		return fmt.Sprintf("%s: no eligible nodes (evaluated=%d filtered=%d)",
			tg, metric.NodesEvaluated, metric.NodesFiltered)
	}
	return fmt.Sprintf("%s: coalescedFailures=%d nodesEvaluated=%d nodesFiltered=%d",
		tg, metric.CoalescedFailures, metric.NodesEvaluated, metric.NodesFiltered)
}

// filterReasons 汇总节点被过滤的具体原因（约束 / 节点类），如 "missing drivers"。
// 只给计数（coalescedFailures=1 nodesFiltered=1）看不出为什么调度失败；
// 可行动的原因在 AllocationMetric.ConstraintFiltered / ClassFiltered 里。
func filterReasons(m *api.AllocationMetric) []string {
	reasons := make([]string, 0, len(m.ConstraintFiltered)+len(m.ClassFiltered))
	for reason, n := range m.ConstraintFiltered {
		if n > 0 {
			reasons = append(reasons, formatFilterReason(reason, n))
		}
	}
	for class, n := range m.ClassFiltered {
		if n > 0 {
			reasons = append(reasons, fmt.Sprintf("node class %s: %d", class, n))
		}
	}
	sort.Strings(reasons)
	return reasons
}

// formatFilterReason 把 Nomad 过滤键转成可行动摘要。
// "missing drivers" 只给计数时用户看不出要装 Docker 还是改用 exec。
func formatFilterReason(reason string, n int) string {
	base := fmt.Sprintf("%s: %d", reason, n)
	if reason == "missing drivers" {
		return base + " — required task driver not detected on filtered nodes (Docker catalog apps need Docker Engine + healthy docker client plugin; or deploy with exec/raw_exec)"
	}
	return base
}

// ListAllocTaskEvents 返回 alloc 各任务的近期事件（启动失败诊断）。
func ListAllocTaskEvents(ctx context.Context, client *api.Client, allocID string) ([]AllocTaskEvent, error) {
	alloc, _, err := client.Allocations().Info(allocID, (&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get alloc %s: %w", allocID, err)
	}
	if alloc == nil {
		return nil, fmt.Errorf("alloc %s not found", allocID)
	}
	var out []AllocTaskEvent
	for task, st := range alloc.TaskStates {
		if st == nil {
			continue
		}
		for _, ev := range st.Events {
			if ev == nil {
				continue
			}
			msg := ev.DisplayMessage
			if msg == "" {
				msg = ev.Details["message"]
				if msg == "" {
					msg = ev.Type
				}
			}
			out = append(out, AllocTaskEvent{
				Task:    task,
				Type:    ev.Type,
				Time:    ev.Time / int64(time.Millisecond),
				Message: msg,
				Fails:   ev.FailsTask,
			})
		}
	}
	// 按时间升序
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Time < out[i].Time {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// AllocLogsOpts 是 GetAllocLogs 的入参。Timeout/MaxBytes 为 0 时使用默认值。
type AllocLogsOpts struct {
	AllocID  string
	Task     string
	LogType  string
	Timeout  time.Duration
	MaxBytes int
}

// GetAllocLogs 拉取任务 stdout/stderr 快照（follow=false，从末尾向前）。
func GetAllocLogs(ctx context.Context, client *api.Client, opts AllocLogsOpts) (*AllocLogsResult, error) {
	if strings.TrimSpace(opts.AllocID) == "" {
		return nil, fmt.Errorf("alloc id is required")
	}
	allocID := opts.AllocID
	logType := strings.ToLower(strings.TrimSpace(opts.LogType))
	if logType != "stdout" && logType != "stderr" {
		logType = "stdout"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultLogTimeout
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultLogMaxBytes
	}

	alloc, _, err := client.Allocations().Info(allocID, (&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get alloc %s: %w", allocID, err)
	}
	if alloc == nil {
		return nil, fmt.Errorf("alloc %s not found", allocID)
	}
	task := strings.TrimSpace(opts.Task)
	if task == "" {
		for name := range alloc.TaskStates {
			task = name
			break
		}
		if task == "" && alloc.Job != nil {
			for _, tg := range alloc.Job.TaskGroups {
				if tg == nil || (alloc.TaskGroup != "" && strDeref(tg.Name) != alloc.TaskGroup) {
					continue
				}
				for _, tsk := range tg.Tasks {
					if tsk != nil {
						task = tsk.Name
						break
					}
				}
			}
		}
	}
	if task == "" {
		return nil, fmt.Errorf("alloc %s has no tasks", allocID)
	}

	cancel := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(cancel) }) }
	defer stop()

	frames, errs := client.AllocFS().Logs(alloc, false, task, logType, "end", int64(maxBytes), cancel, (&api.QueryOptions{}).WithContext(ctx))

	res, err := readLogs(logsSource{frames: frames, errs: errs}, maxBytes, time.After(timeout), stop)
	if err != nil {
		return nil, fmt.Errorf("alloc logs %s/%s: %w", allocID, task, err)
	}
	res.AllocID = allocID
	res.Task = task
	res.LogType = logType
	return res, nil
}

// logsSource 抽象日志流的 frames/errs 通道，便于用普通 channel 单测 readLogs。
type logsSource struct {
	frames <-chan *api.StreamFrame
	errs   <-chan error
}

// readLogs 从日志流读取结果。maxBytes 超限或 timeout 触发时置 Truncated；
// stop 用于取消 SDK 侧流。errs 关闭（EOF）或收到非 nil 错误时终止读取。
func readLogs(src logsSource, maxBytes int, timeout <-chan time.Time, stop func()) (*AllocLogsResult, error) {
	var b strings.Builder
	truncated := false
loop:
	for {
		select {
		case fr, ok := <-src.frames:
			if !ok {
				break loop
			}
			if fr == nil {
				continue
			}
			if len(fr.Data) > 0 {
				if b.Len()+len(fr.Data) > maxBytes {
					remain := maxBytes - b.Len()
					if remain > 0 {
						b.Write(fr.Data[:remain])
					}
					truncated = true
					stop()
					break loop
				}
				b.Write(fr.Data)
			}
		case e, ok := <-src.errs:
			if !ok {
				// errs 关闭（EOF 信号）：流结束，不再可能有错误。
				break loop
			}
			if e != nil {
				stop()
				return nil, fmt.Errorf("stream error: %w", e)
			}
			// nil error：SDK 的同步刷新信号，继续读 frames。
		case <-timeout:
			stop()
			truncated = true
			break loop
		}
	}
	return &AllocLogsResult{
		Content:   b.String(),
		Truncated: truncated,
	}, nil
}
