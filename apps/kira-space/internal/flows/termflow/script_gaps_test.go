package termflow_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// J-AUTO-001: params from a schedule reach the process environment byte for byte, and the run log
// is stored and survives a restart.
func TestScriptParamsReachProcess(t *testing.T) {
	app, clock := clockApp(t)
	out := t.TempDir()
	awkward := `it's "quoted" $HOME \ back  spaced`
	rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) {
		f.Command = `printf '%s' "$KIRA_PARAM_WHO" > ` + out + `/who` + "\n" +
			`printf '%s' "$KIRA_PARAM_PICK" > ` + out + `/pick` + "\n" +
			`printf '%s' "$KIRA_PARAM_ALL" > ` + out + `/all` + "\n" +
			`echo "who=$KIRA_PARAM_WHO"`
		f.Params = []scripts.Param{
			{Name: "who", Type: scripts.ParamText, Required: true},
			{Name: "pick", Type: scripts.ParamSelect, Options: []string{"a", "b"}, Default: []string{"a"}},
			{Name: "all", Type: scripts.ParamMultiselect, Options: []string{"x", "y", "z"}},
		}
		f.Schedule.Params = map[string][]string{"who": {awkward}, "pick": {"b"}, "all": {"x", "z"}}
	})
	advance(t, clock, 30*time.Second)
	run := waitSched(t, app, rec.ID, 0, "done")

	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if got := read("who"); got != awkward {
		t.Fatalf("KIRA_PARAM_WHO = %q, want %q", got, awkward)
	}
	if got := read("pick"); got != "b" {
		t.Fatalf("KIRA_PARAM_PICK = %q, want b", got)
	}
	all := read("all")
	if !strings.Contains(all, "x") || !strings.Contains(all, "z") || strings.Contains(all, "y") {
		t.Fatalf("KIRA_PARAM_ALL = %q, want x and z only", all)
	}
	before := logText(t, app, run.ID)
	if !strings.Contains(before, "who="+awkward) {
		t.Fatalf("log = %q, want the echoed value", before)
	}

	app.Restart()
	if after := logText(t, app, run.ID); after != before {
		t.Fatalf("log after Restart = %q, want %q", after, before)
	}
}

// J-AUTO-002: the schedule fires once per tick across a restart, and a pause skips ticks with no
// catch-up when it ends.
func TestScheduleTicksRestartAndPause(t *testing.T) {
	app, clock := clockApp(t)
	rec := recurring(t, app, "* * * * *", nil)
	advance(t, clock, 30*time.Second)
	waitSched(t, app, rec.ID, 0, "done")

	rearmed(t, clock)
	app.Restart()
	// The closed scheduler's timer stays registered; the new one is the second.
	if !clock.BlockUntil(2, wait) {
		t.Fatal("the restarted scheduler never armed a timer")
	}
	clock.Advance(time.Minute)
	waitSched(t, app, rec.ID, 1, "done")
	if n := len(scheduledRuns(t, app, rec.ID)); n != 2 {
		t.Fatalf("runs after a restart and two ticks = %d, want 2 (none missed or doubled)", n)
	}

	paused := update(t, app, rec, func(f *scripts.CustomScriptFields) { f.Schedule.Enabled = false })
	clock.Advance(5 * time.Minute)
	update(t, app, paused, func(f *scripts.CustomScriptFields) { f.Schedule.Enabled = true })
	rearmed(t, clock)
	advance(t, clock, time.Minute)
	waitSched(t, app, rec.ID, 2, "done")
	if n := len(scheduledRuns(t, app, rec.ID)); n != 3 {
		t.Fatalf("runs after a pause = %d, want 3 (the paused ticks never fired)", n)
	}
}

// J-BC-026: ReadLog keeps the newest 5000 lines of a run and resumes by afterSeq. At exact multiples
// of the cap nothing is lost or repeated, one past it drops the oldest line and says so.
func TestRunLogPaging(t *testing.T) {
	const limit = 5000
	app, clock := clockApp(t)
	rec := recurring(t, app, "* * * * *", nil)
	cur := rec
	for i, n := range []int{limit - 1, limit, limit + 1, 2 * limit} {
		cmd := fmt.Sprintf("seq 1 %d", n)
		mark := app.Events.Mark()
		cur = update(t, app, cur, func(f *scripts.CustomScriptFields) { f.Command = cmd })
		waitSnapshot(t, app, mark, func(sn scripts.Snapshot) bool { r, _ := scriptOf(sn, rec.ID); return r.Command == cmd })
		if i == 0 {
			advance(t, clock, 30*time.Second)
		} else {
			rearmed(t, clock)
			advance(t, clock, time.Minute)
		}
		run := waitSched(t, app, rec.ID, i, "done")

		got, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: run.ID})
		if err != nil {
			t.Fatal(err)
		}
		first := max(1, n-limit+1)
		if want := n - first + 1; len(got.Chunks) != want || got.Truncated != (n > limit) {
			t.Fatalf("n=%d: %d lines, truncated %v; want %d lines, truncated %v", n, len(got.Chunks), got.Truncated, want, n > limit)
		}
		for j, c := range got.Chunks {
			if c.Text != fmt.Sprint(first+j) || (j > 0 && c.Seq != got.Chunks[j-1].Seq+1) {
				t.Fatalf("n=%d: chunk %d = %+v, want line %d in order", n, j, c, first+j)
			}
		}
		last := got.Chunks[len(got.Chunks)-1].Seq
		if rest, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: run.ID, AfterSeq: last}); err != nil || len(rest.Chunks) != 0 {
			t.Fatalf("n=%d: a read after the last line = %+v, %v, want none", n, rest, err)
		}
		mid := got.Chunks[len(got.Chunks)/2].Seq
		tail, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: run.ID, AfterSeq: mid})
		if err != nil || len(tail.Chunks) == 0 || tail.Chunks[0].Seq != mid+1 || tail.Chunks[len(tail.Chunks)-1].Seq != last {
			t.Fatalf("n=%d: a resume after seq %d = %v chunks, %v", n, mid, len(tail.Chunks), err)
		}
	}
}

// J-AUTO-004: a bad run file beside good ones is reported with its line; the good entries stay.
func TestRunsBadFileBesideGood(t *testing.T) {
	app := flowharness.New(t)
	mark := app.Events.Mark()
	writeRun(t, app, "good.yaml", "name: Good\ncolor: blue\ncommand: echo ok\n")
	writeRun(t, app, "other.yaml", "name: Other\ncolor: blue\ncommand: echo ok\n")
	writeRun(t, app, "broken.yaml", "name: Broken\ncolor: blue\nkind: bogus\ncommand: echo no\n")
	snap := waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool {
		return len(s.Scripts) == 2 && len(s.Problems) == 1
	})
	p := snap.Problems[0]
	if p.File != "broken.yaml" || p.Line != 3 || p.LastGood {
		t.Fatalf("problem = %+v, want broken.yaml line 3 with no last-good record", p)
	}
	if _, ok := scriptOf(snap, "broken"); ok {
		t.Fatal("the broken file is listed as a script")
	}
	for _, id := range []string{"good", "other"} {
		if _, ok := scriptOf(snap, id); !ok {
			t.Fatalf("good entry %q was dropped: %+v", id, snap.Scripts)
		}
	}
}
