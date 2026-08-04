package metrics

import (
	"testing"

	"github.com/nphq/np/internal/nomad"
)

func TestAppendSample_RingBuffer(t *testing.T) {
	var s []nomad.LoadSample
	for i := int64(1); i <= 70; i++ {
		s = appendSample(s, nomad.LoadSample{Time: i, CPU: float64(i)}, 60)
	}
	if len(s) != 60 {
		t.Fatalf("len = %d, want 60", len(s))
	}
	if s[0].Time != 11 {
		t.Fatalf("first = %d, want 11 (oldest 10 dropped)", s[0].Time)
	}
	if s[59].Time != 70 {
		t.Fatalf("last = %d, want 70", s[59].Time)
	}
}

func TestAppendSample_BelowCap(t *testing.T) {
	var s []nomad.LoadSample
	for i := int64(1); i <= 3; i++ {
		s = appendSample(s, nomad.LoadSample{Time: i}, 5)
	}
	if len(s) != 3 || s[0].Time != 1 {
		t.Fatalf("got %+v", s)
	}
}

func TestPercentToMHz(t *testing.T) {
	cases := []struct {
		pct, total, want float64
	}{
		{50, 3200, 1600},
		{100, 800, 800},
		{0, 3200, 0},
		{50, 0, 0}, // 无容量信息 → 0
	}
	for _, c := range cases {
		if got := percentToMHz(c.pct, c.total); got != c.want {
			t.Errorf("percentToMHz(%v, %v) = %v, want %v", c.pct, c.total, got, c.want)
		}
	}
}

func TestNodeLoadChanged_Thresholds(t *testing.T) {
	base := nomad.NodeLoad{Available: true, RunningAllocs: 2,
		Used: nomad.ResourceUsage{CPU: 100, Memory: 100, Disk: 100}}
	if nodeLoadChanged(base, base) {
		t.Error("same load should not be changed")
	}
	small := base
	small.Used.CPU = 100.4 // <1MHz
	if nodeLoadChanged(base, small) {
		t.Error("sub-threshold CPU delta should not be changed")
	}
	big := base
	big.Used.Memory = 200
	if !nodeLoadChanged(base, big) {
		t.Error("memory jump should be changed")
	}
	flip := base
	flip.Available = false
	if !nodeLoadChanged(base, flip) {
		t.Error("available flip should be changed")
	}
	capCh := base
	capCh.Capacity = nomad.ResourceUsage{CPU: 999}
	if !nodeLoadChanged(base, capCh) {
		t.Error("capacity change should be changed")
	}
}

func TestAllocLoadChanged(t *testing.T) {
	base := nomad.AllocLoad{AllocID: "a", Tasks: map[string]nomad.TaskUsage{
		"web": {CPU: 100, Memory: 50},
	}}
	if allocLoadChanged(base, base) {
		t.Error("same should not be changed")
	}
	same := base
	same.Tasks = map[string]nomad.TaskUsage{"web": {CPU: 100.3, Memory: 50}}
	if allocLoadChanged(base, same) {
		t.Error("sub-threshold should not be changed")
	}
	diff := base
	diff.Tasks = map[string]nomad.TaskUsage{"web": {CPU: 300, Memory: 50}}
	if !allocLoadChanged(base, diff) {
		t.Error("cpu jump should be changed")
	}
	extra := base
	extra.Tasks = map[string]nomad.TaskUsage{"web": {CPU: 100, Memory: 50}, "sidecar": {CPU: 5}}
	if !allocLoadChanged(base, extra) {
		t.Error("task set change should be changed")
	}
}
