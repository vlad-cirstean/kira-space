//go:build !darwin || !cgo

package metrics

import "github.com/shirou/gopsutil/v4/process"

// listProcesses is AppProcessSet's process-table source on every platform without a native
// enumeration syscall wired up (or without cgo to reach the one on darwin) — unchanged from before
// the darwin/other split: process.Processes() then Exe() and Ppid() per process (D3).
func listProcesses() ([]procEntry, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	entries := make([]procEntry, 0, len(procs))
	for _, p := range procs {
		// An unreadable exe keeps the entry (exe ""): a parent chain must not break.
		exe, _ := p.Exe()
		ppid, err := p.Ppid()
		if err != nil {
			continue
		}
		entries = append(entries, procEntry{pid: p.Pid, ppid: ppid, exe: exe})
	}
	return entries, nil
}
