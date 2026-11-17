// Package metrics is the Go analogue of Electron's app.getAppMetrics(): it sums memory and CPU
// across the app's whole process set on a timer, and it is also the instrument gate
// G1 is measured with. The memory figure is RSS everywhere except darwin, where it is
// ri_phys_footprint (proc_pid_rusage(RUSAGE_INFO_V2)) — the figure Activity Monitor's own "Memory"
// column shows, which excludes shared system pages RSS double-counts across this app's own
// multi-process set. cmd/g1measure, this package's own throwaway RSS-only measurement
// tool, is unaffected — see its own header comment.
//
// The process set is not obvious and getting it wrong invalidates any measurement built on this
// package. It has three disjoint groups. App: a native webview's helper processes (WebKitGTK's
// WebProcess/NetworkProcess on Linux, WKWebView's com.apple.WebKit.* helpers on macOS) are not
// children of this process in the ppid sense, so the app is matched by executable path substring.
// Agents: the descendant trees of the root pids registered in procroots.Agents (the caller
// passes them in), walked over the one process table listed per tick. An agent pid never also
// counts as App. Scripts: the descendant trees of root pids in procroots.Scripts, minus agent pids.
package metrics

import (
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Sample mirrors packages/shared/protocol/events.ts's AppMetricsSample shape. CPUPercent is
// normalized to the machine's whole capacity (0-100, occasionally a hair over from measurement
// jitter), not the per-core-sum a tool like `top` reports per process (which can read e.g. 350% on
// a busy quad-core machine) — StatusBar.vue renders it as a plain "N%" with no further context or
// clamping (and reserves only 4 characters of layout width for it, "100%"), so a raw per-core-sum
// would both mean the wrong thing to a reader and overflow that reserved width. MemoryBytes is RSS
// everywhere except darwin, where it is phys_footprint — see this package's own doc comment.
// LogicalCPUs and ProcessCount exist so a renderer can state what CPUPercent is a
// percentage *of* and how many processes MemoryBytes covers, rather than leaving a reader to take
// either figure on faith — ProcessCount is the number of pids that answered the probe this tick
// (the ones MemoryBytes is summed over), not the number discovered.
type Sample struct {
	CPUPercent   float64 `json:"cpuPercent"`  // App + Agents + Scripts, clamped 0..100
	MemoryBytes  uint64  `json:"memoryBytes"` // App + Agents + Scripts
	LogicalCPUs  int     `json:"logicalCPUs"`
	ProcessCount int     `json:"processCount"` // App + Agents + Scripts
	App          Usage   `json:"app"`
	Agents       Usage   `json:"agents"`
	Scripts      Usage   `json:"scripts"`
}

// Usage is one process group's share of a Sample.
type Usage struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemoryBytes  uint64  `json:"memoryBytes"`
	ProcessCount int     `json:"processCount"`
}

// ProcessSet is one tick's process groups. A pid is in at most one.
type ProcessSet struct{ App, Agents, Scripts []int32 }

// cpuState is one pid's cumulative CPU time, tagged with the OS process-creation time it was read
// from — the tag is what lets two samples of "the same pid" be told apart from two samples of two
// different processes that happen to share a pid number: a short-lived helper can exit and
// have its pid recycled to an unrelated new process well within one sample interval, and comparing
// cumulative times across that boundary produces a meaningless (often negative) delta.
type cpuState struct {
	time       float64
	createTime int64
}

// procSample is one probe's answer for a single pid: the process-identity tag (createTime, same
// role as cpuState's) plus the two readings Sample needs from it. Sampler.probe is the seam every
// platform's syscalls sit behind, so Sample itself, cpuDeltaPercent and CachedPIDs never call an
// OS API directly — see probe_darwin.go/probe_other.go.
type procSample struct {
	cpuSeconds float64
	memBytes   uint64
	createTime int64
}

// cpuSanityThresholdPercent is cpuDeltaPercent's own sanity bound: a normalized reading above this
// before clamping means an accounting bug (a stale-baseline spike, a bad probe read), not
// measurement jitter, and is worth a log line even though the emitted sample still gets clamped to
// a displayable range.
const cpuSanityThresholdPercent = 110

// Sampler keeps the previous CPU-time sample so CPUPercent reports a live delta (matching
// today's cpu.percentCPUUsage semantics) rather than gopsutil's own cumulative-since-start
// figure, normalized by the machine's own logical core count so it lands in the same 0-100 range
// regardless of how many cores the process set is spread across.
type Sampler struct {
	// pids also returns any procSample its own call
	// already obtained for a subset of the returned pids — CachedPIDs.PIDs's own rescan path
	// probes every pid to get its createTime anyway, so Sample below reuses that result directly
	// instead of calling probe a second time for the same pid on the same tick. nil/empty is
	// always a valid answer (a revalidate-only tick has no fresh probe data to offer; any
	// pids-only caller can return nil), so this is additive, not a stricter contract.
	pids        func() (ProcessSet, map[int32]procSample, error)
	probe       func(pid int32) (procSample, bool)
	prevCPU     map[int32]cpuState
	prevAt      time.Time
	logicalCPUs int
}

// NewSampler takes a pid-discovery function so the caller decides how the process set is found —
// see Sum below for the bundle-matching implementation this app uses. The per-process
// probe itself is always defaultProbe (platform-selected, see probe_darwin.go/probe_other.go) —
// there is no production caller that needs a different one, so it is not part of this signature.
func NewSampler(pids func() (ProcessSet, map[int32]procSample, error)) *Sampler {
	return &Sampler{pids: pids, probe: defaultProbe, prevCPU: map[int32]cpuState{}, logicalCPUs: runtime.NumCPU()}
}

func (s *Sampler) Sample() (Sample, error) {
	set, fresh, err := s.pids()
	if err != nil {
		return Sample{}, err
	}

	// A failed probe drops a pid from both readings for the tick rather than contributing a
	// zero: cpuState.time is cumulative, so a zero fed into next tick's delta would read as this
	// pid's entire lifetime CPU usage in one window (F2, D7).
	collect := func(ids []int32) (Usage, map[int32]cpuState) {
		var u Usage
		now := make(map[int32]cpuState, len(ids))
		for _, pid := range ids {
			ps, ok := fresh[pid]
			if !ok {
				ps, ok = s.probe(pid)
			}
			if !ok {
				continue
			}
			u.MemoryBytes += ps.memBytes
			u.ProcessCount++
			now[pid] = cpuState{time: ps.cpuSeconds, createTime: ps.createTime}
		}
		return u, now
	}
	app, appNow := collect(set.App)
	agents, agentNow := collect(set.Agents)
	scripts, scriptNow := collect(set.Scripts)

	now := time.Now()
	if !s.prevAt.IsZero() {
		elapsed := now.Sub(s.prevAt).Seconds()
		app.CPUPercent = cpuDeltaPercent(s.prevCPU, appNow, elapsed, s.logicalCPUs)
		agents.CPUPercent = cpuDeltaPercent(s.prevCPU, agentNow, elapsed, s.logicalCPUs)
		scripts.CPUPercent = cpuDeltaPercent(s.prevCPU, scriptNow, elapsed, s.logicalCPUs)
	}
	cpuNow := make(map[int32]cpuState, len(appNow)+len(agentNow)+len(scriptNow))
	for _, m := range []map[int32]cpuState{appNow, agentNow, scriptNow} {
		for pid, c := range m {
			cpuNow[pid] = c
		}
	}
	s.prevCPU = cpuNow
	s.prevAt = now

	return Sample{
		CPUPercent:   min(app.CPUPercent+agents.CPUPercent+scripts.CPUPercent, 100),
		MemoryBytes:  app.MemoryBytes + agents.MemoryBytes + scripts.MemoryBytes,
		LogicalCPUs:  s.logicalCPUs,
		ProcessCount: app.ProcessCount + agents.ProcessCount + scripts.ProcessCount,
		App:          app,
		Agents:       agents,
		Scripts:      scripts,
	}, nil
}

// cpuDeltaPercent is Sample's own delta math, pulled out as a pure function so the pid-reuse
// guard and the core-count normalization are both unit-testable without a real OS process: a pid
// only contributes if it named the same process (matching createTime) in both snapshots — a pid
// missing from prev (a genuinely new process, including one whose probe failed last tick — see
// Sample's handling above) or whose createTime changed (an exited process's pid reused by an
// unrelated new one) contributes nothing for this tick, rather than a delta computed against a
// stranger's cumulative CPU time. A per-pid delta that goes backwards (a non-monotonic read) is
// clamped to 0 rather than subtracted from the other pids' genuine usage, and the normalized
// result is clamped to [0, 100] — a raw value above cpuSanityThresholdPercent is logged once with
// the pids that produced it, since that means an accounting bug worth seeing rather than jitter to
// round away silently.
func cpuDeltaPercent(prev, cur map[int32]cpuState, elapsedSeconds float64, logicalCPUs int) float64 {
	if elapsedSeconds <= 0 || logicalCPUs <= 0 {
		return 0
	}
	var deltaSum float64
	var contributors []int32
	for pid, c := range cur {
		p, ok := prev[pid]
		if !ok || p.createTime != c.createTime {
			continue
		}
		delta := (c.time - p.time) / elapsedSeconds
		if delta < 0 {
			delta = 0
		}
		deltaSum += delta
		if delta > 0 {
			contributors = append(contributors, pid)
		}
	}

	raw := deltaSum * 100 / float64(logicalCPUs)
	if raw > cpuSanityThresholdPercent {
		slog.Warn("cpu sample exceeded sanity threshold, clamping", "scope", "metrics", "percent", raw, "pids", contributors)
	}
	switch {
	case raw < 0:
		return 0
	case raw > 100:
		return 100
	default:
		return raw
	}
}

// procEntry is one listProcesses result: a pid paired with its parent pid and executable path. Kept
// platform-independent so listProcesses.go/processlist_other.go's darwin/other split is only about
// *how* the process table is enumerated — AppProcessSet's own matching loop below never changes.
type procEntry struct {
	pid  int32
	ppid int32
	exe  string
}

// AppProcessSet finds this app's own process set: pids matching anchorNeedles directly (this
// app's own executable, which lives under a bundle path unique to this app), plus pids matching
// helperNeedles (e.g. "com.apple.WebKit" for WKWebView's XPC helpers) that are actually this app's
// own helpers — not simply every process on the machine whose executable happens to match the
// substring.
//
// Why the helper/anchor split exists: a native webview's helper processes are not children of
// this app in the ppid sense (WKWebView's are reparented to launchd, ppid=1), so a plain substring
// match over-matches — confirmed on a real machine measuring P52 gate G1: "com.apple.WebKit"
// matched Messages' and Notes' own idle WebContent/GPU/Networking helpers too, inflating a 215 MB
// reading to 300 MB. On darwin, macOS itself tracks which process is "responsible" for launching
// each XPC service (the same mechanism Activity Monitor uses to group them visually) via
// responsibility_get_pid_responsible_for_pid, so helper pids are kept only when that resolves to
// one of the anchor pids. On every other platform responsiblePID returns -1 (unknown), so helper
// pids are kept unfiltered — correct there specifically because P52 §2.3 already confirmed
// WebKitGTK's own helpers really are ppid children on Linux, so this over-match risk doesn't arise.
//
// Scans the system's process table exactly once, regardless of how many needles are given (P57
// finding: an earlier version called MatchingPIDs — a full process.Processes() enumeration plus
// an Exe() syscall on every process on the machine — once per needle, i.e. once per *element* of
// AnchorNeedles/HelperNeedles combined. With this app's own needle lists that's several full
// system-wide scans every 5s tick for the life of the process, almost all of it re-deriving the
// exact same process-to-executable-path facts over and over. One scan, checked against every
// needle per process, produces the identical result for a small, fixed syscall cost instead of one
// that scales with the needle count).
//
// listProcesses is platform-selected (processlist_darwin.go's proc_listpids+proc_pidpath, or
// processlist_other.go's gopsutil process.Processes()+Exe() — P7 F8): the matching rule below is
// unaffected by which one supplied procEntry's pid/exe pairs.
func AppProcessSet(anchorNeedles, helperNeedles []string) ([]int32, error) {
	set, err := ProcessSets(anchorNeedles, helperNeedles, nil, nil)
	return set.App, err
}

// ProcessSets lists the process table once and splits it into the app group (AppProcessSet's
// rule) and the agent group: every root in agentRoots still in the table, plus all its
// descendants, and the script group likewise for scriptRoots. Precedence Agents > Scripts > App: a
// sandbox helper (bwrap) inside an agent tree is counted once, as an agent, and a pid claimed by
// agents is never also a script.
func ProcessSets(anchorNeedles, helperNeedles []string, agentRoots, scriptRoots []int32) (ProcessSet, error) {
	entries, err := listProcesses()
	if err != nil {
		return ProcessSet{}, err
	}
	return groupProcesses(entries, anchorNeedles, helperNeedles, agentRoots, scriptRoots, responsibilityTracked, responsiblePID), nil
}

// groupProcesses is ProcessSets' pure core: tree walk plus classification precedence.
func groupProcesses(entries []procEntry, anchorNeedles, helperNeedles []string, agentRoots, scriptRoots []int32, tracked bool, responsible func(int32) int32) ProcessSet {
	children := make(map[int32][]int32, len(entries))
	present := make(map[int32]bool, len(entries))
	for _, e := range entries {
		present[e.pid] = true
		children[e.ppid] = append(children[e.ppid], e.pid)
	}

	// BFS with a visited set: a ppid cycle from a pid-reuse race must terminate.
	claimed := map[int32]bool{}
	walk := func(roots []int32) []int32 {
		var out, queue []int32
		for _, r := range roots {
			if present[r] && !claimed[r] {
				claimed[r] = true
				queue = append(queue, r)
			}
		}
		for len(queue) > 0 {
			pid := queue[0]
			queue = queue[1:]
			out = append(out, pid)
			for _, c := range children[pid] {
				if !claimed[c] {
					claimed[c] = true
					queue = append(queue, c)
				}
			}
		}
		return out
	}
	agents := walk(agentRoots)
	scripts := walk(scriptRoots)

	var anchors, helpers []int32
	for _, e := range entries {
		if e.exe == "" || claimed[e.pid] {
			continue
		}
		switch {
		case containsAny(e.exe, anchorNeedles):
			anchors = append(anchors, e.pid)
		case containsAny(e.exe, helperNeedles):
			helpers = append(helpers, e.pid)
		}
	}

	anchorSet := make(map[int32]bool, len(anchors))
	for _, pid := range anchors {
		anchorSet[pid] = true
	}
	app := append([]int32{}, anchors...)
	for _, pid := range helpers {
		if includeHelper(tracked, anchorSet[responsible(pid)]) {
			app = append(app, pid)
		}
	}
	return ProcessSet{App: app, Agents: agents, Scripts: scripts}
}

// includeHelper is AppProcessSet's own inclusion rule, pulled out pure and platform-independent
// (responsibilityTracked is the only thing that varies by platform) so both branches are
// unit-testable everywhere: untracked platforms (every non-darwin build) always include a
// helper-needle match unfiltered, since those are real ppid children
// with no cross-app over-match risk. A tracked platform (darwin) must fail closed instead: a -1
// "no distinct responsible process" answer from responsiblePID is exactly the
// belongs-to-some-other-app case this whole mechanism exists to exclude (the previous
// `resp == -1 || anchorSet[resp]` unconditionally included that case too, defeating the check it
// sat right next to).
func includeHelper(tracked, isAnchor bool) bool {
	if !tracked {
		return true
	}
	return isAnchor
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// CachedPIDs wraps a pid-resolving func (AppProcessSet, in production) so a caller that samples
// on a fixed cadence for the life of the process — main.go's own metrics ticker — doesn't walk
// every process on the machine on every single tick: resolve only runs once every
// rescanEvery calls; between full resolves, the pids it found are kept as long as they still name
// the same OS process (a fresh, targeted process.NewProcess(pid).CreateTime() lookup — cheap
// compared to resolve's own Exe() lookup on every process on the machine — still matches what was
// recorded at the last full resolve). A pid whose process has exited, possibly with the pid number
// already reused by something else entirely, is dropped rather than kept or misattributed, and is
// picked up again (correctly, whatever it now maps to) on the next resolve.
//
// Deliberately never reuses a *process.Process handle across calls to re-check CreateTime:
// gopsutil's own CreateTime caches its answer on the Process value forever after the first
// successful call, so a long-lived handle for an exited pid would keep reporting its old answer
// rather than surfacing that the pid died — only a fresh process.NewProcess(pid) queries the OS
// again.
type CachedPIDs struct {
	resolve     func() (ProcessSet, error)
	rescanEvery int
	sinceScan   int
	createTimes map[int32]int64
	group       map[int32]procGroup
}

type procGroup uint8

const (
	groupApp procGroup = iota
	groupAgents
	groupScripts
)

// NewCachedPIDs builds a CachedPIDs. rescanEvery must be at least 1 (a full resolve every call —
// pass 1 for that, not 0).
func NewCachedPIDs(resolve func() (ProcessSet, error), rescanEvery int) *CachedPIDs {
	return &CachedPIDs{resolve: resolve, rescanEvery: rescanEvery, createTimes: map[int32]int64{}, group: map[int32]procGroup{}}
}

// PIDs is the func a Ticker/Sampler calls each tick.
// PIDs is the func a Ticker/Sampler calls each tick — see Sampler.pids' own doc comment for why
// it also returns a procSample map: on a rescan tick,
// resolve's own createTime check already probed every pid once, so this hands that same result to
// Sample rather than making it probe the identical pid a second time in the same tick. A
// revalidate-only tick (sinceScan != 0) has no fresh probe data of its own to offer — revalidate
// only confirms createTime, cheaper than a full probe, and does not read memory/CPU — so it
// returns nil, which Sample already treats as "probe this pid yourself".
func (c *CachedPIDs) PIDs() (ProcessSet, map[int32]procSample, error) {
	var fresh map[int32]procSample
	if c.sinceScan == 0 {
		samples, err := c.rescan()
		if err != nil {
			return ProcessSet{}, nil, err
		}
		fresh = samples
	} else {
		c.revalidate()
	}
	c.sinceScan = (c.sinceScan + 1) % c.rescanEvery

	var out ProcessSet
	for pid := range c.createTimes {
		switch c.group[pid] {
		case groupAgents:
			out.Agents = append(out.Agents, pid)
		case groupScripts:
			out.Scripts = append(out.Scripts, pid)
		default:
			out.App = append(out.App, pid)
		}
	}
	return out, fresh, nil
}

func (c *CachedPIDs) rescan() (map[int32]procSample, error) {
	set, err := c.resolve()
	if err != nil {
		return nil, err
	}
	n := len(set.App) + len(set.Agents) + len(set.Scripts)
	createTimes := make(map[int32]int64, n)
	samples := make(map[int32]procSample, n)
	group := make(map[int32]procGroup, n)
	probe := func(pids []int32, g procGroup) {
		for _, pid := range pids {
			if ps, ok := defaultProbe(pid); ok {
				createTimes[pid] = ps.createTime
				samples[pid] = ps
				group[pid] = g
			}
		}
	}
	probe(set.App, groupApp)
	probe(set.Agents, groupAgents)
	probe(set.Scripts, groupScripts)
	c.createTimes = createTimes
	c.group = group
	return samples, nil
}

func (c *CachedPIDs) revalidate() {
	for pid, want := range c.createTimes {
		got, ok := processCreateTime(pid)
		if !ok || got != want {
			delete(c.createTimes, pid)
			delete(c.group, pid)
		}
	}
}

// processCreateTime reads createTime off the same probe Sampler uses (rather than its own
// NewProcess/CreateTime lookup), so CachedPIDs' idea of "the same process" and cpuState's can
// never diverge — the create-time tag both use to detect pid reuse always comes from one place.
// defaultProbe is platform-selected: probe_darwin.go (native, requires cgo) or probe_other.go
// (gopsutil, every other build).
func processCreateTime(pid int32) (int64, bool) {
	ps, ok := defaultProbe(pid)
	if !ok {
		return 0, false
	}
	return ps.createTime, true
}

// gopsutilProbe is the gopsutil-based per-process probe: NewProcess (an existing-pid check),
// MemoryInfo (RSS) and Times (cumulative CPU seconds), folded into one pass/fail result rather than
// letting the memory and CPU reads fail independently. probe_other.go's defaultProbe is this
// unchanged, on every platform without a native footprint syscall (or without cgo to reach the one
// that exists); probe_darwin.go's EPERM fallback also calls gopsutilCreateTime below for its
// identity tag, since struct proc_taskinfo has no start-time field of its own (D3).
// Consumed only by probe_other.go (!darwin || !cgo) — invisible to a darwin+cgo lint run.
//
//nolint:unused // build-tag-only consumer, see above
func gopsutilProbe(pid int32) (procSample, bool) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return procSample{}, false
	}
	mi, err := p.MemoryInfo()
	if err != nil || mi == nil {
		return procSample{}, false
	}
	times, err := p.Times()
	if err != nil {
		return procSample{}, false
	}
	createTime, err := p.CreateTime()
	if err != nil {
		return procSample{}, false
	}
	return procSample{cpuSeconds: times.User + times.System, memBytes: mi.RSS, createTime: createTime}, true
}

// gopsutilCreateTime is process-creation-time alone (gopsutil's CreateTime, a sysctl kern.proc.pid
// on darwin) — probe_darwin.go's EPERM-fallback identity tag, kept separate from gopsutilProbe so
// that fallback isn't tied to gopsutil's own unchecked-return MemoryInfo/Times reads (the exact
// pattern F2 flags as a bug) when all it actually needs is the creation timestamp.
// Consumed only by probe_darwin.go (darwin && cgo) — invisible to a default-target lint run.
//
//nolint:unused // build-tag-only consumer, see above
func gopsutilCreateTime(pid int32) (int64, bool) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return 0, false
	}
	ct, err := p.CreateTime()
	if err != nil {
		return 0, false
	}
	return ct, true
}
