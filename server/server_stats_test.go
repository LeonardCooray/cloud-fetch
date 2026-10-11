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
	free, ok := s.loadStats(dir, &sync.Mutex{})

	if !ok || free != s.DiskFree {
		t.Fatalf("loadStats returned (%d, %v), want (%d, true)", free, ok, s.DiskFree)
	}
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

func TestLoadStatsReportsUnreadableDisk(t *testing.T) {
	s := stats{pusher: &countPusher{}}
	if _, ok := s.loadStats("/no/such/dir", &sync.Mutex{}); ok {
		t.Fatal("loadStats reported free space for a missing directory")
	}
}
