package metrics

import (
	"os"
	"os/exec"
	"testing"
)

// cpuDeltaPercent is the pure delta math cpuState/Sample pull apart to test the P2 R1 pid-reuse
// guard and core-count normalization without a real OS process: a pid contributes to the delta
// only when its createTime is unchanged between the two snapshots, and the raw per-core-sum delta
// is divided by logicalCPUs before it comes back. logicalCPUs: 1 below is a no-op divisor, isolating
// each of those tests to the pid-reuse behavior alone; normalization itself gets its own tests.

func TestCpuDeltaPercent_SamePidSameProcess_ContributesDelta(t *testing.T) {
	prev := map[int32]cpuState{100: {time: 1.0, createTime: 5000}}
	cur := map[int32]cpuState{100: {time: 1.5, createTime: 5000}}
	got := cpuDeltaPercent(prev, cur, 1.0, 1)
	if got != 50 {
		t.Errorf("cpuDeltaPercent = %v, want 50 (0.5s of CPU time over 1s elapsed)", got)
	}
}

func TestCpuDeltaPercent_ReusedPidDifferentCreateTime_ContributesNothing(t *testing.T) {
	// pid 100 named a process that had accumulated 100s of CPU time, then exited; the OS handed
	// pid 100 to a brand-new, unrelated process before the next sample. Computing a delta against
	// the old process's cumulative time would produce a large, spurious negative contribution.
	prev := map[int32]cpuState{100: {time: 100.0, createTime: 5000}}
	cur := map[int32]cpuState{100: {time: 0.1, createTime: 9999}}
	got := cpuDeltaPercent(prev, cur, 1.0, 1)
	if got != 0 {
		t.Errorf("cpuDeltaPercent = %v, want 0 (reused pid must not mix the old process's cumulative time into the delta)", got)
	}
}

func TestCpuDeltaPercent_NewPid_ContributesNothingThisTick(t *testing.T) {
	prev := map[int32]cpuState{}
	cur := map[int32]cpuState{200: {time: 0.3, createTime: 1234}}
	got := cpuDeltaPercent(prev, cur, 1.0, 1)
	if got != 0 {
		t.Errorf("cpuDeltaPercent = %v, want 0 (a pid with no prior sample has no delta to report yet)", got)
	}
}

func TestCpuDeltaPercent_ZeroElapsed_ReturnsZero(t *testing.T) {
	prev := map[int32]cpuState{100: {time: 1.0, createTime: 5000}}
	cur := map[int32]cpuState{100: {time: 2.0, createTime: 5000}}
	if got := cpuDeltaPercent(prev, cur, 0, 1); got != 0 {
		t.Errorf("cpuDeltaPercent = %v, want 0 for zero elapsed seconds (avoid a divide-by-zero blowup)", got)
	}
}

// Regression: StatusBar.vue renders CPUPercent as a plain "N%" with no clamping and a
// 4-character reserved width ("100%") — the raw per-core-sum a multi-process, multi-core-busy app
// produces (e.g. 350 on a quad-core machine fully loaded) must be normalized to the machine's own
// 0-100 range before it ever reaches that layer.
func TestCpuDeltaPercent_NormalizesByLogicalCPUCount(t *testing.T) {
	// Two full cores' worth of CPU time accumulated across the process set over 1 elapsed second
	// — a raw per-core-sum of 200 — on a 4-logical-core machine should read 50%, not 200%.
	prev := map[int32]cpuState{100: {time: 0, createTime: 1}, 200: {time: 0, createTime: 2}}
	cur := map[int32]cpuState{100: {time: 1.0, createTime: 1}, 200: {time: 1.0, createTime: 2}}
	got := cpuDeltaPercent(prev, cur, 1.0, 4)
	if got != 50 {
		t.Errorf("cpuDeltaPercent = %v, want 50 (200%% raw per-core-sum / 4 logical cores)", got)
	}
}

func TestCpuDeltaPercent_ZeroLogicalCPUs_ReturnsZero(t *testing.T) {
	prev := map[int32]cpuState{100: {time: 1.0, createTime: 5000}}
	cur := map[int32]cpuState{100: {time: 2.0, createTime: 5000}}
	if got := cpuDeltaPercent(prev, cur, 1.0, 0); got != 0 {
		t.Errorf("cpuDeltaPercent = %v, want 0 for a zero core count (avoid a divide-by-zero blowup)", got)
	}
}

// The clamp/monotonicity rules — the one test this phase's refactor earns
// per CLAUDE.md's bar, since each subtest below guards a rule nothing else in the package checks:
// a per-pid delta cannot go negative and drag down another pid's genuine usage, a pid recovering
// from a probe failure cannot manufacture a lifetime-sized spike out of a stale baseline it no
// longer has, and the normalized result can never leave the range StatusBar.vue renders unclamped.
func TestCpuDeltaPercent_ClampingRules(t *testing.T) {
	t.Run("BackwardsCounterClampedNotSubtracted", func(t *testing.T) {
		// pid 100's next reading goes backwards (a bad probe read, not a real CPU-time decrease);
		// pid 200 gains a genuine 1s of CPU time in the same window. The backwards delta must clamp
		// to 0 for pid 100 rather than being subtracted from the total, or one bad pid could mask
		// another pid's genuine usage.
		prev := map[int32]cpuState{100: {time: 10.0, createTime: 5000}, 200: {time: 0.0, createTime: 6000}}
		cur := map[int32]cpuState{100: {time: 8.0, createTime: 5000}, 200: {time: 1.0, createTime: 6000}}
		got := cpuDeltaPercent(prev, cur, 1.0, 4)
		if got != 25 {
			t.Errorf("cpuDeltaPercent = %v, want 25 (pid 100's backwards delta clamped to 0, pid 200's genuine 1s / 4 cores = 25%%)", got)
		}
	})

	t.Run("RecoveredAfterProbeFailureNoStaleBaselineSpike", func(t *testing.T) {
		// A pid whose probe failed last tick is dropped from Sample's cpuNow entirely (see Sample's
		// probe loop), so it is missing from prev here exactly like a brand-new pid — it must not
		// be differenced against a stale cumulative reading from before the failure, which would
		// read as that pid's entire lifetime CPU usage compressed into one sample window.
		prev := map[int32]cpuState{} // dropped last tick when its probe failed
		cur := map[int32]cpuState{100: {time: 300.0, createTime: 7000}}
		got := cpuDeltaPercent(prev, cur, 1.0, 1)
		if got != 0 {
			t.Errorf("cpuDeltaPercent = %v, want 0 (a recovered pid has no prior sample to delta against, same as a genuinely new pid)", got)
		}
	})

	t.Run("ExtremeRawValueClampedTo100", func(t *testing.T) {
		// The normalized result must never leave [0, 100] even when the raw per-core-sum is far
		// outside it (a stale-baseline spike, or several busy cores) — StatusBar.vue renders it
		// unclamped into a fixed-width slot.
		prev := map[int32]cpuState{100: {time: 0, createTime: 1}}
		cur := map[int32]cpuState{100: {time: 1000.0, createTime: 1}}
		got := cpuDeltaPercent(prev, cur, 1.0, 1)
		if got != 100 {
			t.Errorf("cpuDeltaPercent = %v, want 100 (clamped, not left at the raw 100000%%)", got)
		}
	})
}

// includeHelper is AppProcessSet's own inclusion rule, pulled out pure so both the tracked
// (darwin) and untracked (every other platform) branches are testable regardless of which
// platform actually runs this test binary.

func TestIncludeHelper_TrackedFailsClosedOnUnknownResponsible(t *testing.T) {
	if includeHelper(true, false) {
		t.Error("tracked platform: a helper whose responsible pid is not one of our own anchors must be excluded, not included")
	}
}

func TestIncludeHelper_TrackedIncludesOwnAnchor(t *testing.T) {
	if !includeHelper(true, true) {
		t.Error("tracked platform: a helper whose responsible pid is one of our own anchors must be included")
	}
}

func TestIncludeHelper_UntrackedAlwaysIncludes(t *testing.T) {
	if !includeHelper(false, false) {
		t.Error("untracked platform: every helper-needle match must be included unfiltered, responsibility tracking not being available there")
	}
}

// CachedPIDs: resolve (the expensive full process-table walk) should run once up front and then
// only every rescanEvery calls — everything in between must be answered from cheap per-pid
// revalidation instead.

func TestCachedPIDs_ResolveOnlyRunsEveryRescanEvery(t *testing.T) {
	self := int32(os.Getpid())
	resolveCalls := 0
	c := NewCachedPIDs(func() (ProcessSet, error) {
		resolveCalls++
		return ProcessSet{App: []int32{self}}, nil
	}, 3)

	for i := 1; i <= 7; i++ {
		pids, _, err := c.PIDs()
		if err != nil {
			t.Fatalf("PIDs() call %d: %v", i, err)
		}
		if len(pids.App) != 1 || pids.App[0] != self {
			t.Fatalf("PIDs() call %d = %v, want [%d]", i, pids, self)
		}
	}
	if resolveCalls != 3 {
		t.Errorf("resolveCalls = %d, want 3 (once at call 1, then every 3rd call: 4 and 7)", resolveCalls)
	}
}

func TestCachedPIDs_DropsExitedPidBeforeNextRescan(t *testing.T) {
	self := int32(os.Getpid())

	// A short sleep, not an instantly-exiting command: the child must still be alive when the
	// first PIDs() call (the full rescan) runs, or this test would not be exercising what it
	// claims to.
	cmd := exec.Command("sleep", "0.3")
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start a throwaway child process: %v", err)
	}
	childPID := int32(cmd.Process.Pid)

	c := NewCachedPIDs(func() (ProcessSet, error) {
		return ProcessSet{App: []int32{self, childPID}}, nil
	}, 10) // large enough that the second PIDs() call below stays on the cheap revalidation path

	first, _, err := c.PIDs()
	if err != nil {
		t.Fatalf("PIDs() (rescan): %v", err)
	}
	if !containsPID(first.App, self) || !containsPID(first.App, childPID) {
		t.Fatalf("PIDs() (rescan) = %v, want both %d and %d (child must still be alive here)", first, self, childPID)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("child process exited with error: %v", err)
	}
	exitedPID := childPID

	second, _, err := c.PIDs()
	if err != nil {
		t.Fatalf("PIDs() (revalidate): %v", err)
	}
	if !containsPID(second.App, self) {
		t.Errorf("PIDs() (revalidate) = %v, want %d (still alive) kept", second, self)
	}
	if containsPID(second.App, exitedPID) {
		t.Errorf("PIDs() (revalidate) = %v, want exited pid %d dropped, not kept or misattributed to whatever now has that pid number", second, exitedPID)
	}
}

func containsPID(pids []int32, want int32) bool {
	for _, p := range pids {
		if p == want {
			return true
		}
	}
	return false
}

// With RescanEvery == 1 (the production value), every
// single tick used to probe each pid twice — once inside CachedPIDs.PIDs' own rescan (to read
// createTime), and again inside Sampler.Sample (to read memory/CPU) — even though rescan's own
// probe already read every field Sample needs. Sample now reuses whatever procSample its own
// `pids` func hands back for a pid, falling back to `probe` only for a pid missing from that map.
// This test drives Sampler.Sample directly (bypassing CachedPIDs and any real OS process) with a
// `pids` func that returns a `fresh` map covering every pid, and asserts `probe` is never called —
// it fails against the pre-fix Sample (which called probe unconditionally for every pid) and
// passes once the `fresh` map is checked first.
func TestSampler_Sample_ReusesFreshProcSamples_NoDoubleProbe(t *testing.T) {
	freshSamples := map[int32]procSample{
		1: {cpuSeconds: 1.5, memBytes: 1000, createTime: 100},
		2: {cpuSeconds: 2.5, memBytes: 2000, createTime: 200},
	}
	probeCalls := 0
	s := &Sampler{
		pids: func() (ProcessSet, map[int32]procSample, error) {
			return ProcessSet{App: []int32{1}, Agents: []int32{2}}, freshSamples, nil
		},
		probe: func(pid int32) (procSample, bool) {
			probeCalls++
			t.Errorf("probe(%d) called even though pids() already supplied a fresh procSample for it", pid)
			return procSample{}, false
		},
		prevCPU:     map[int32]cpuState{},
		logicalCPUs: 1,
	}

	sample, err := s.Sample()
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if probeCalls != 0 {
		t.Errorf("probe was called %d time(s), want 0", probeCalls)
	}
	if sample.App.MemoryBytes != 1000 || sample.Agents.MemoryBytes != 2000 {
		t.Errorf("App/Agents MemoryBytes = %d/%d, want 1000/2000", sample.App.MemoryBytes, sample.Agents.MemoryBytes)
	}
	if sample.MemoryBytes != 3000 {
		t.Errorf("MemoryBytes = %d, want 3000 (1000 + 2000, from the fresh samples)", sample.MemoryBytes)
	}
	if sample.ProcessCount != 2 {
		t.Errorf("ProcessCount = %d, want 2", sample.ProcessCount)
	}
}

// A pid missing from the fresh map (a revalidate-only tick, which has no fresh data at all — see
// PIDs' own doc comment) must still fall back to probe.
func TestSampler_Sample_FallsBackToProbeWhenNoFreshSample(t *testing.T) {
	probeCalls := 0
	s := &Sampler{
		pids: func() (ProcessSet, map[int32]procSample, error) {
			return ProcessSet{App: []int32{7}}, nil, nil // nil fresh map: the revalidate-tick shape
		},
		probe: func(pid int32) (procSample, bool) {
			probeCalls++
			return procSample{cpuSeconds: 0.5, memBytes: 500, createTime: 42}, true
		},
		prevCPU:     map[int32]cpuState{},
		logicalCPUs: 1,
	}

	sample, err := s.Sample()
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if probeCalls != 1 {
		t.Errorf("probe was called %d time(s), want 1 (no fresh sample was offered for pid 7)", probeCalls)
	}
	if sample.MemoryBytes != 500 {
		t.Errorf("MemoryBytes = %d, want 500", sample.MemoryBytes)
	}
}

// Tree walk plus classification precedence: a slip double counts or drops a group silently.
func TestGroupProcesses(t *testing.T) {
	const app = "/x/Kira Space"
	noResp := func(int32) int32 { return -1 }
	tests := []struct {
		name        string
		entries     []procEntry
		roots       []int32
		scriptRoots []int32
		wantApp     []int32
		wantAgents  []int32
		wantScripts []int32
	}{
		{
			name: "descendants at depth 3",
			entries: []procEntry{
				{pid: 10, ppid: 1, exe: "/bin/zsh"}, {pid: 11, ppid: 10}, {pid: 12, ppid: 11, exe: "/bin/node"},
				{pid: 13, ppid: 12, exe: "/bin/sh"}, {pid: 99, ppid: 1, exe: "/bin/other"},
			},
			roots:      []int32{10},
			wantAgents: []int32{10, 11, 12, 13},
		},
		{
			name:    "root missing from table ignored",
			entries: []procEntry{{pid: 20, ppid: 1, exe: "/bin/zsh"}},
			roots:   []int32{555},
		},
		{
			name:       "ppid cycle terminates",
			entries:    []procEntry{{pid: 30, ppid: 31, exe: "/bin/a"}, {pid: 31, ppid: 30, exe: "/bin/b"}},
			roots:      []int32{30},
			wantAgents: []int32{30, 31},
		},
		{
			name: "helper in agent tree is an agent, anchor outside stays app",
			entries: []procEntry{
				{pid: 40, ppid: 1, exe: app}, {pid: 50, ppid: 1, exe: "/bin/zsh"}, {pid: 51, ppid: 50, exe: "/usr/bin/bwrap"},
			},
			roots:      []int32{50},
			wantApp:    []int32{40},
			wantAgents: []int32{50, 51},
		},
		{
			name: "script tree at depth 2",
			entries: []procEntry{
				{pid: 60, ppid: 1, exe: "/bin/sh"}, {pid: 61, ppid: 60, exe: "/bin/sleep"}, {pid: 62, ppid: 61, exe: "/bin/x"},
			},
			scriptRoots: []int32{60},
			wantScripts: []int32{60, 61, 62},
		},
		{
			name: "script root inside agent tree stays agent",
			entries: []procEntry{
				{pid: 70, ppid: 1, exe: "/bin/zsh"}, {pid: 71, ppid: 70, exe: "/bin/sh"},
			},
			roots:       []int32{70},
			scriptRoots: []int32{71},
			wantAgents:  []int32{70, 71},
		},
		{
			name: "agent root inside script tree stays agent, siblings stay script",
			entries: []procEntry{
				{pid: 80, ppid: 1, exe: "/bin/sh"}, {pid: 81, ppid: 80, exe: "/bin/claude"}, {pid: 82, ppid: 81, exe: "/bin/x"},
				{pid: 83, ppid: 80, exe: "/bin/sleep"},
			},
			roots:       []int32{81},
			scriptRoots: []int32{80},
			wantAgents:  []int32{81, 82},
			wantScripts: []int32{80, 83},
		},
		{
			name: "helper in script tree is a script",
			entries: []procEntry{
				{pid: 40, ppid: 1, exe: app}, {pid: 90, ppid: 1, exe: "/bin/sh"}, {pid: 91, ppid: 90, exe: "/usr/bin/bwrap"},
			},
			scriptRoots: []int32{90},
			wantApp:     []int32{40},
			wantScripts: []int32{90, 91},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupProcesses(tt.entries, []string{"Kira Space"}, HelperNeedles, tt.roots, tt.scriptRoots, false, noResp)
			if !sameSet(got.App, tt.wantApp) || !sameSet(got.Agents, tt.wantAgents) || !sameSet(got.Scripts, tt.wantScripts) {
				t.Errorf("got App=%v Agents=%v Scripts=%v, want App=%v Agents=%v Scripts=%v", got.App, got.Agents, got.Scripts, tt.wantApp, tt.wantAgents, tt.wantScripts)
			}
		})
	}
}

func sameSet(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for _, p := range b {
		if !containsPID(a, p) {
			return false
		}
	}
	return true
}
