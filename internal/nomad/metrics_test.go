package nomad

import (
	"math"
	"testing"

	"github.com/hashicorp/nomad/api"
)

func TestDiskUsedMiB_PrefersAllocDirStats(t *testing.T) {
	hs := &api.HostStats{
		AllocDirStats: &api.HostDiskStats{
			Size: 1000240963584,
			Used: 782563893248,
		},
		DiskStats: []*api.HostDiskStats{
			{Device: "/dev/disk1s1s1", Mountpoint: "/", Size: 1000240963584, Used: 782563893248},
			{Device: "/dev/disk1s2", Mountpoint: "/System/Volumes/Data", Size: 1000240963584, Used: 782563893248},
			{Device: "/dev/disk1s3", Mountpoint: "/System/Volumes/Preboot", Size: 1000240963584, Used: 782563893248},
			{Device: "OrbStack", Mountpoint: "/Users/x/OrbStack", Size: 359100000000, Used: 161135000000},
		},
	}
	got := diskUsedMiB(hs)
	want := float64(782563893248) / bytesPerMiB
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("diskUsedMiB = %v, want %v (AllocDirStats)", got, want)
	}
	// 旧逻辑求和会到 ~4TB / capacity ~1TB → 400%+
	var sum float64
	for _, d := range hs.DiskStats {
		sum += float64(d.Used) / 1e6
	}
	if sum < want*3 {
		t.Fatalf("sanity: naive sum %v should greatly exceed AllocDir used %v", sum, want)
	}
}

func TestDiskUsedMiB_FallbackRootMount(t *testing.T) {
	hs := &api.HostStats{
		DiskStats: []*api.HostDiskStats{
			{Mountpoint: "/boot", Size: 1 << 30, Used: 100 << 20},
			{Mountpoint: "/", Size: 100 << 30, Used: 40 << 30},
			{Mountpoint: "/data", Size: 50 << 30, Used: 10 << 30},
		},
	}
	got := diskUsedMiB(hs)
	want := float64(40<<30) / bytesPerMiB
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("diskUsedMiB = %v, want root %v", got, want)
	}
}

func TestDiskUsedMiB_FallbackLargestWhenNoRoot(t *testing.T) {
	hs := &api.HostStats{
		DiskStats: []*api.HostDiskStats{
			{Mountpoint: "/small", Size: 10 << 30, Used: 1 << 30},
			{Mountpoint: "/big", Size: 100 << 30, Used: 25 << 30},
			{Mountpoint: "/zero", Size: 0, Used: 0},
		},
	}
	got := diskUsedMiB(hs)
	want := float64(25<<30) / bytesPerMiB
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("diskUsedMiB = %v, want largest %v", got, want)
	}
}
