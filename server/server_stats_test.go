package server

import (
	"sync"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

type countPusher struct{ n int }

func (p *countPusher) Push() bool { p.n++; return true }

// DiskFree is what the service user can still write, so it must be the
// filesystem's available figure, never Total-Used (which includes root's reserve).
func TestLoadStatsReportsAvailableDisk(t *testing.T) {
	dir := t.TempDir()
	p := &countPusher{}
	s := stats{pusher: p}
	s.loadStats(dir, &sync.Mutex{})

	if !s.Set || p.n != 1 {
		t.Fatalf("stats not set and pushed: set=%v pushes=%d", s.Set, p.n)
	}
	if s.DiskFree <= 0 || s.DiskFree > s.DiskTotal-s.DiskUsed {
		t.Fatalf("diskFree=%d, want 0 < free <= total-used (%d)", s.DiskFree, s.DiskTotal-s.DiskUsed)
	}
	u, err := disk.Usage(dir)
	if err != nil {
		t.Fatal(err)
	}
	// other processes write in between, so allow some drift
	if d := int64(u.Free) - s.DiskFree; d > 1<<30 || d < -(1<<30) {
		t.Fatalf("diskFree=%d, disk.Usage Free=%d", s.DiskFree, u.Free)
	}
}
