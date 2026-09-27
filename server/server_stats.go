package server

import (
	"runtime"
	"sync"

	velox "github.com/jpillora/velox/go"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type stats struct {
	Set         bool    `json:"set"`
	CPU         float64 `json:"cpu"`
	DiskUsed    int64   `json:"diskUsed"`
	DiskTotal   int64   `json:"diskTotal"`
	MemoryUsed  int64   `json:"memoryUsed"`
	MemoryTotal int64   `json:"memoryTotal"`
	GoMemory    int64   `json:"goMemory"`
	GoRoutines  int     `json:"goRoutines"`
	//internal
	pusher velox.Pusher
}

// loadStats gathers fresh numbers, then swaps them in under l (the state
// lock velox marshals under), so a push never reads a half-written struct.
func (s *stats) loadStats(diskDir string, l sync.Locker) {
	next := stats{Set: true, pusher: s.pusher}
	//count cpu cycles between last count
	if percents, err := cpu.Percent(0, false); err == nil && len(percents) == 1 {
		next.CPU = percents[0]
	}
	//count disk usage
	if stat, err := disk.Usage(diskDir); err == nil {
		next.DiskUsed = int64(stat.Used)
		next.DiskTotal = int64(stat.Total)
	}
	//count memory usage
	if stat, err := mem.VirtualMemory(); err == nil {
		next.MemoryUsed = int64(stat.Used)
		next.MemoryTotal = int64(stat.Total)
	}
	//count total bytes allocated by the go runtime
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	next.GoMemory = int64(memStats.Alloc)
	//count current number of goroutines
	next.GoRoutines = runtime.NumGoroutine()
	l.Lock()
	*s = next
	l.Unlock()
	s.pusher.Push()
}
