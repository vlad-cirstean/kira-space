package adeflow_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/pprofserve"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P693 RAM probe, Go side. Opt-in: KIRA_RAM_PROBE=1. Mirrors tests/perf/heap-ram.spec.ts `ade-agents`
// (10 live tasks, 200 archived, 4 running TUI sessions fed redraw frames). Per checkpoint it prints
// `GORAM <cp> <json>`, appends the record to <out>/space-go.jsonl and writes heap-<cp>.pb.gz and
// goroutine-<cp>.txt. KIRA_RAM_OUT: output dir, relative paths resolve against the repo root.
// docs/DEV_ENVIRONMENT.md "Perf probes".

const (
	ramLive     = 10
	ramArchived = 200
	ramRunning  = 4
	ramFrameMB  = 2
)

// ramClass: PSS splits shared pages (the re-executed test binary behind claude-fake shares text with
// the app), so AnonMb (private anonymous memory) is kept as the stable companion figure.
type ramClass struct {
	Count  int     `json:"count"`
	PssMb  float64 `json:"pssMb"`
	AnonMb float64 `json:"anonMb"`
}

type ramRecord struct {
	Checkpoint string              `json:"checkpoint"`
	HeapAlloc  float64             `json:"heapAllocMb"`
	HeapInuse  float64             `json:"heapInuseMb"`
	StackInuse float64             `json:"stackInuseMb"`
	Sys        float64             `json:"sysMb"`
	NumGC      uint32              `json:"numGC"`
	Goroutines int                 `json:"goroutines"`
	Classes    map[string]ramClass `json:"classes"`
	Peak       map[string]ramClass `json:"peak,omitempty"`
}

func mb(b uint64) float64 { return float64(b) / (1 << 20) }

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func procPPID(pid int) (int, bool) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	s := string(raw)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(f[1])
	return ppid, err == nil
}

func procPss(pid int) (pss, anon float64) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/smaps_rollup")
	if err != nil {
		return 0, 0
	}
	for _, l := range strings.Split(string(raw), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		kb, _ := strconv.ParseFloat(f[1], 64)
		switch f[0] {
		case "Pss:":
			pss = kb / 1024
		case "Anonymous:":
			anon = kb / 1024
		}
	}
	return pss, anon
}

// classify names one descendant: shell (login shell a terminal runs), claude-fake (the re-executed
// test binary standing in for claude), git (cat-file split out), js-worker, other.
func classify(pid, self int, selfExe string) string {
	if pid == self {
		return "app"
	}
	comm, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	cmdRaw, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	name := strings.TrimSpace(string(comm))
	cmd := strings.ReplaceAll(string(cmdRaw), "\x00", " ")
	exe, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	switch {
	case strings.Contains(cmd, "js-worker"):
		return "js-worker"
	case name == "git" && strings.Contains(cmd, "cat-file"):
		return "git-cat-file"
	case name == "git":
		return "git-other"
	case exe == selfExe || strings.HasPrefix(cmd, "claude"):
		return "claude-fake"
	case name == "zsh" || name == "bash" || name == "sh" || name == "fish":
		return "shell"
	}
	return "other"
}

// processTable sums PSS per class over self and every descendant.
func processTable() map[string]ramClass {
	self := os.Getpid()
	selfExe, _ := os.Readlink("/proc/self/exe")
	ents, _ := os.ReadDir("/proc")
	parent := map[int]int{}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if ppid, ok := procPPID(pid); ok {
			parent[pid] = ppid
		}
	}
	out := map[string]ramClass{}
	for pid := range parent {
		mine := pid == self
		for p := pid; !mine && p > 1; p = parent[p] {
			mine = parent[p] == self
		}
		if !mine {
			continue
		}
		c := classify(pid, self, selfExe)
		cur := out[c]
		cur.Count++
		pss, anon := procPss(pid)
		cur.PssMb += pss
		cur.AnonMb += anon
		out[c] = cur
	}
	for k, v := range out {
		v.PssMb, v.AnonMb = round2(v.PssMb), round2(v.AnonMb)
		out[k] = v
	}
	return out
}

type ramProbe struct {
	t   *testing.T
	out string
}

func (p *ramProbe) checkpoint(cp string, peak map[string]ramClass) {
	p.t.Helper()
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rec := ramRecord{
		Checkpoint: cp, HeapAlloc: round2(mb(m.HeapAlloc)), HeapInuse: round2(mb(m.HeapInuse)),
		StackInuse: round2(mb(m.StackInuse)), Sys: round2(mb(m.Sys)), NumGC: m.NumGC,
		Goroutines: runtime.NumGoroutine(), Classes: processTable(), Peak: peak,
	}
	heap, err := os.Create(filepath.Join(p.out, "heap-"+cp+".pb.gz"))
	if err != nil {
		p.t.Fatal(err)
	}
	if err := pprof.Lookup("heap").WriteTo(heap, 0); err != nil {
		p.t.Fatal(err)
	}
	_ = heap.Close()
	gr, err := os.Create(filepath.Join(p.out, "goroutine-"+cp+".txt"))
	if err != nil {
		p.t.Fatal(err)
	}
	if err := pprof.Lookup("goroutine").WriteTo(gr, 1); err != nil {
		p.t.Fatal(err)
	}
	_ = gr.Close()
	line, _ := json.Marshal(rec)
	fmt.Printf("GORAM %s %s\n", cp, line)
	f, err := os.OpenFile(filepath.Join(p.out, "space-go.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		p.t.Fatal(err)
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
}

// samplePeaks polls the process table every 500 ms until stop is called; stop returns the per-class
// peak count and peak PSS (they can come from different samples).
func samplePeaks() (stop func() map[string]ramClass) {
	var mu sync.Mutex
	peak := map[string]ramClass{}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				tbl := processTable()
				mu.Lock()
				for k, v := range tbl {
					cur := peak[k]
					cur.Count = max(cur.Count, v.Count)
					cur.PssMb = max(cur.PssMb, v.PssMb)
					cur.AnonMb = max(cur.AnonMb, v.AnonMb)
					peak[k] = cur
				}
				mu.Unlock()
			}
		}
	}()
	return func() map[string]ramClass {
		close(done)
		wg.Wait()
		return peak
	}
}

// ramFrames writes about `mb` MB of Claude-like full-screen redraws (ED2, home, 40 colored lines of
// 190 columns; the shape of tests/perf/support/ramWorlds.ts agentFrames) and returns the path.
func ramFrames(t *testing.T, dir string, megabytes int) string {
	t.Helper()
	var b strings.Builder
	for f := 0; b.Len() < megabytes<<20; f++ {
		b.WriteString("\x1b[2J\x1b[H")
		for l := 0; l < 40; l++ {
			text := fmt.Sprintf("agent frame %d line %d ", f, l)
			text += strings.Repeat(strconv.Itoa(l%10), max(0, 190-len(text)))
			fmt.Fprintf(&b, "\x1b[3%dm%s\x1b[0m\r\n", l%6+1, text)
		}
	}
	path := filepath.Join(dir, "frames.bin")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func ramOutDir(t *testing.T) string {
	t.Helper()
	out := os.Getenv("KIRA_RAM_OUT")
	root, err := filepath.Abs("../../../../..")
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case out == "":
		out = filepath.Join(root, "apps/kira-space/perf-ram/go", strconv.FormatInt(time.Now().Unix(), 10))
	case !filepath.IsAbs(out):
		out = filepath.Join(root, out)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRAMProbeAgents(t *testing.T) {
	if os.Getenv("KIRA_RAM_PROBE") != "1" {
		t.Skip("set KIRA_RAM_PROBE=1 to run the P693 Go RAM probe")
	}
	runtime.MemProfileRate = 32 << 10
	if os.Getenv("KIRA_PPROF") != "" {
		pprofserve.Start()
	}
	probe := &ramProbe{t: t, out: ramOutDir(t)}
	dir := t.TempDir()
	never := filepath.Join(dir, "never")
	t.Cleanup(func() { _ = os.WriteFile(never, nil, 0o644) })
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	claude(app, map[string][]fakeagent.Action{"*": {{Emit: ramFrames(t, dir, ramFrameMB), WaitFile: never}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	board(t, app)
	probe.checkpoint("boot", nil)

	live := make([]adewire.Task, ramLive)
	for i := range live {
		live[i] = createTask(t, app, "Live task "+strconv.Itoa(i), "flow", rec.ID)
	}
	for i := 0; i < ramArchived; i++ {
		tk := createTask(t, app, "Archived task "+strconv.Itoa(i)+": ship the billing export", "flow", rec.ID)
		if err := app.W.AdeTask.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: tk.ID}); err != nil {
			t.Fatalf("ArchiveTask %d: %v", i, err)
		}
	}
	b := board(t, app)
	t.Logf("board: %d tasks, %d history", len(b.Tasks), len(b.History))
	probe.checkpoint("board200", nil)

	sessions := make([]adewire.Launch, ramRunning)
	for i := range sessions {
		br := branchOf(t, b, live[i].ID, rec.ID)
		named, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: br.ID, Name: "feat/ram-" + strconv.Itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		l, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: named.ID})
		if err != nil {
			t.Fatal(err)
		}
		openLaunch(t, app, l)
		sessions[i] = l
	}
	for _, l := range sessions {
		waitSession(t, app, l.SessionID, "running")
	}
	probe.checkpoint("sessions4", nil)

	time.Sleep(60 * time.Second)
	probe.checkpoint("idle60", nil)

	stop := samplePeaks()
	for i := 0; i < 50; i++ {
		est := strconv.Itoa(i%7+1) + "h"
		if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: live[4].ID, Patch: adewire.TaskPatch{Est: &est}}); err != nil {
			t.Fatal(err)
		}
		board(t, app)
		if _, err := app.W.AdeTask.Sessions(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
	}
	probe.checkpoint("pushes50", stop())

	for _, l := range sessions {
		if err := app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: l.SessionID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range sessions {
		waitSession(t, app, l.SessionID, "stopped")
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return processTable()["claude-fake"].Count == 0 })
	probe.checkpoint("stopped4", nil)

	time.Sleep(10 * time.Second)
	probe.checkpoint("final", nil)
	keys := make([]string, 0)
	for k := range processTable() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("process classes at end: %v", keys)
}
