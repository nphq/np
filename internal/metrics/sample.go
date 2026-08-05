package metrics

import (
	"math"
	"time"

	"github.com/nphq/np/internal/nomad"
)

// 采样与 diff 的纯函数（全部可单测）。

// msNow 返回毫秒时间戳。
func msNow() int64 { return time.Now().UnixMilli() }

// nowFunc 可被测试覆写（环形缓冲/去重逻辑需要确定性的时间）。
var nowFunc = msNow

// usageSample 把用量打包成采样点。
func usageSample(t int64, u nomad.ResourceUsage) nomad.LoadSample {
	return nomad.LoadSample{Time: t, CPU: u.CPU, Memory: u.Memory, Disk: u.Disk}
}

// appendSample 是环形缓冲：满 maxLen 丢最旧。
func appendSample(samples []nomad.LoadSample, s nomad.LoadSample, maxLen int) []nomad.LoadSample {
	if maxLen <= 0 {
		maxLen = 60
	}
	if len(samples) < maxLen {
		return append(samples, s)
	}
	n := len(samples)
	if n == 0 {
		return []nomad.LoadSample{s}
	}
	out := make([]nomad.LoadSample, n, maxLen)
	copy(out, samples[1:])
	out[n-1] = s
	return out
}

// usageDelta 是两个用量三元组的绝对差（用于 diff 判定）。
func usageDelta(a, b nomad.ResourceUsage) (cpu, mem, disk float64) {
	return math.Abs(a.CPU - b.CPU), math.Abs(a.Memory - b.Memory), math.Abs(a.Disk - b.Disk)
}

// nodeLoadChanged 判定节点负载是否需要发事件。
// 阈值：CPU 1MHz / 内存 1MB / 磁盘 1MB；available / runningAllocs 翻转也发。
func nodeLoadChanged(prev, next nomad.NodeLoad) bool {
	if prev.Available != next.Available || prev.RunningAllocs != next.RunningAllocs {
		return true
	}
	if prev.Capacity != next.Capacity || prev.Allocated != next.Allocated {
		return true
	}
	cpu, mem, disk := usageDelta(prev.Used, next.Used)
	return cpu > 1 || mem > 1 || disk > 1
}

// allocLoadChanged 判定 alloc 是否需要发事件（CPU 1MHz / 内存 1MB）。
func allocLoadChanged(prev, next nomad.AllocLoad) bool {
	if len(prev.Tasks) != len(next.Tasks) {
		return true
	}
	for name, nt := range next.Tasks {
		pt, ok := prev.Tasks[name]
		if !ok {
			return true
		}
		if math.Abs(pt.CPU-nt.CPU) > 1 || math.Abs(pt.Memory-nt.Memory) > 1 {
			return true
		}
	}
	return false
}
