package nomad

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/nomad/api"
)

// GetEvaluation 查询评估状态（部署进度 / blocked 原因）。
func GetEvaluation(client *api.Client, evalID string) (*EvalInfo, error) {
	if strings.TrimSpace(evalID) == "" {
		return nil, fmt.Errorf("eval id is required")
	}
	ev, _, err := client.Evaluations().Info(evalID, nil)
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
		var parts []string
		for tg, metric := range ev.FailedTGAllocs {
			if metric == nil {
				parts = append(parts, tg+": placement failed")
				continue
			}
			avail := 0
			for _, n := range metric.NodesAvailable {
				avail += n
			}
			if metric.CoalescedFailures == 0 && avail == 0 && metric.NodesEvaluated > 0 {
				parts = append(parts, fmt.Sprintf("%s: no eligible nodes (evaluated=%d filtered=%d)",
					tg, metric.NodesEvaluated, metric.NodesFiltered))
			} else {
				parts = append(parts, fmt.Sprintf("%s: coalescedFailures=%d nodesEvaluated=%d nodesFiltered=%d",
					tg, metric.CoalescedFailures, metric.NodesEvaluated, metric.NodesFiltered))
			}
		}
		out.FailedSummary = strings.Join(parts, "; ")
	}
	return out, nil
}

// ListAllocTaskEvents 返回 alloc 各任务的近期事件（启动失败诊断）。
func ListAllocTaskEvents(client *api.Client, allocID string) ([]AllocTaskEvent, error) {
	alloc, _, err := client.Allocations().Info(allocID, nil)
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

const maxLogBytes = 64 * 1024

// GetAllocLogs 拉取任务 stdout/stderr 快照（follow=false，从末尾向前）。
func GetAllocLogs(client *api.Client, allocID, task, logType string) (*AllocLogsResult, error) {
	if strings.TrimSpace(allocID) == "" {
		return nil, fmt.Errorf("alloc id is required")
	}
	logType = strings.ToLower(strings.TrimSpace(logType))
	if logType != "stdout" && logType != "stderr" {
		logType = "stdout"
	}

	alloc, _, err := client.Allocations().Info(allocID, nil)
	if err != nil {
		return nil, fmt.Errorf("get alloc %s: %w", allocID, err)
	}
	if alloc == nil {
		return nil, fmt.Errorf("alloc %s not found", allocID)
	}
	task = strings.TrimSpace(task)
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

	frames, errs := client.AllocFS().Logs(alloc, false, task, logType, "end", int64(maxLogBytes), cancel, nil)

	var b strings.Builder
	timeout := time.After(8 * time.Second)
	truncated := false
loop:
	for {
		select {
		case fr, ok := <-frames:
			if !ok {
				break loop
			}
			if fr == nil {
				continue
			}
			if len(fr.Data) > 0 {
				if b.Len()+len(fr.Data) > maxLogBytes {
					remain := maxLogBytes - b.Len()
					if remain > 0 {
						b.Write(fr.Data[:remain])
					}
					truncated = true
					stop()
					break loop
				}
				b.Write(fr.Data)
			}
		case e := <-errs:
			if e != nil {
				stop()
				return nil, fmt.Errorf("alloc logs %s/%s: %w", allocID, task, e)
			}
		case <-timeout:
			stop()
			truncated = true
			break loop
		}
	}

	return &AllocLogsResult{
		AllocID:   allocID,
		Task:      task,
		LogType:   logType,
		Content:   b.String(),
		Truncated: truncated,
	}, nil
}
