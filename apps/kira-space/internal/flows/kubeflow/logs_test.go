package kubeflow

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func (e *env) open(window, stream string, mut func(*kubernetes.LogsOpenArgs)) error {
	args := kubernetes.LogsOpenArgs{
		WindowKey: window, StreamID: stream, ClusterID: e.ids["kind-dev"], Namespace: "default", Pod: "web-1",
		Container: "app", Tail: 100,
	}
	if mut != nil {
		mut(&args)
	}
	return e.app.W.Kubernetes.LogsOpen(args)
}

func TestLogsTail(t *testing.T) {
	e := boot(t, nil)
	for i := 1; i <= 102; i++ {
		e.fake.AppendLog("web-1", fmt.Sprintf("extra %d", i))
	}
	mark := e.app.Events.Mark()
	if err := e.open("w1", "s1", nil); err != nil {
		t.Fatal(err)
	}
	lines, errMsg := waitEnded(t, e.app, mark, "w1", "s1")
	if errMsg != "" || len(lines) != 100 || lines[99].Text != "extra 102" || lines[0].Text != "extra 3" {
		t.Fatalf("got %d lines (%q .. %q) err %q, want the newest 100", len(lines), lines[0].Text, lines[len(lines)-1].Text, errMsg)
	}
	e.contract(t, "event:kira:kubernetes:logs", kubernetes.LogsEvent{StreamID: "s1", Lines: lines[:2], Ended: true})

	var q map[string][]string
	for _, r := range e.fake.Requests("/log") {
		q = mustQuery(t, r.Query)
	}
	if q["tailLines"][0] != "100" || q["limitBytes"][0] != "8388608" || q["container"][0] != "app" {
		t.Fatalf("log query = %v", q)
	}
}

func TestLogsOptions(t *testing.T) {
	e := boot(t, nil)

	t.Run("timestamps", func(t *testing.T) {
		mark := e.app.Events.Mark()
		if err := e.open("w1", "ts", func(a *kubernetes.LogsOpenArgs) { a.Timestamps = true }); err != nil {
			t.Fatal(err)
		}
		lines, _ := waitEnded(t, e.app, mark, "w1", "ts")
		if len(lines) != 3 || lines[0].Text != "line one" || lines[0].TS != "2026-01-01T00:00:00.000000000Z" {
			t.Fatalf("lines = %+v", lines)
		}
		e.contract(t, "event:kira:kubernetes:logs:timestamps", kubernetes.LogsEvent{StreamID: "ts", Lines: lines, Ended: true})
	})

	t.Run("previous", func(t *testing.T) {
		mark := e.app.Events.Mark()
		if err := e.open("w1", "prev", func(a *kubernetes.LogsOpenArgs) { a.Previous = true }); err != nil {
			t.Fatal(err)
		}
		lines, _ := waitEnded(t, e.app, mark, "w1", "prev")
		if !slices.Equal(texts(lines), []string{"prev one", "prev two"}) {
			t.Fatalf("lines = %v", texts(lines))
		}
		err := e.open("w1", "prev2", func(a *kubernetes.LogsOpenArgs) { a.Previous, a.Container = true, "sidecar" })
		wantCode(t, err, "E_BAD_REQUEST")
	})

	t.Run("unknown-container", func(t *testing.T) {
		err := e.open("w1", "bad", func(a *kubernetes.LogsOpenArgs) { a.Container = "nope" })
		wantCode(t, err, "E_BAD_REQUEST")
		// The failed open frees its stream id.
		if err := e.open("w1", "bad", nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("contract-errors", func(t *testing.T) {
		wantCode(t, e.open("w1", "t", func(a *kubernetes.LogsOpenArgs) { a.Tail = 7 }), "E_CONTRACT")
		wantCode(t, e.open("", "t", nil), "E_CONTRACT")
	})
}

func TestLogsFollow(t *testing.T) {
	e := boot(t, nil)
	mark := e.app.Events.Mark()
	follow := func(a *kubernetes.LogsOpenArgs) { a.Follow = true }
	if err := e.open("w1", "f1", follow); err != nil {
		t.Fatal(err)
	}
	if got := waitLines(t, e.app, mark, "w1", "f1", 3); len(got) != 3 {
		t.Fatalf("backlog = %v", texts(got))
	}
	wantCode(t, e.open("w1", "f1", follow), "E_CONTRACT")

	e.fake.AppendLog("web-1", "live line")
	got := waitLines(t, e.app, mark, "w1", "f1", 4)
	if got[3].Text != "live line" {
		t.Fatalf("lines = %v", texts(got))
	}

	if err := e.app.W.Kubernetes.LogsClose(kubernetes.StreamArgs{StreamID: "f1"}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, wait, func() bool { return e.fake.FollowEnded() == 1 })
	if _, errMsg := waitEnded(t, e.app, mark, "w1", "f1"); errMsg != "" {
		t.Fatalf("closed stream ended with error %q", errMsg)
	}
}

func TestLogsCloseWindowEndsOnlyItsStreams(t *testing.T) {
	e := boot(t, nil)
	mark := e.app.Events.Mark()
	follow := func(a *kubernetes.LogsOpenArgs) { a.Follow = true }
	for _, w := range []string{"wa", "wb"} {
		if err := e.open(w, "s-"+w, follow); err != nil {
			t.Fatal(err)
		}
		waitLines(t, e.app, mark, w, "s-"+w, 3)
	}
	e.app.CloseWindow("wa")
	testx.WaitUntil(t, wait, func() bool { return e.fake.FollowEnded() == 1 })
	waitEnded(t, e.app, mark, "wa", "s-wa")
	if e.fake.Followers() != 1 {
		t.Fatalf("followers = %d, want the other window's stream still open", e.fake.Followers())
	}
	e.fake.AppendLog("web-1", "after close")
	if got := waitLines(t, e.app, mark, "wb", "s-wb", 4); got[3].Text != "after close" {
		t.Fatalf("lines = %v", texts(got))
	}
}

func TestLogsQuitEndsStreams(t *testing.T) {
	e := boot(t, nil)
	mark := e.app.Events.Mark()
	if err := e.open("w1", "q", func(a *kubernetes.LogsOpenArgs) { a.Follow = true }); err != nil {
		t.Fatal(err)
	}
	waitLines(t, e.app, mark, "w1", "q", 3)
	e.app.Quit(t)
	testx.WaitUntil(t, wait, func() bool { return e.fake.FollowEnded() == 1 })
}

// J-K8S-024: a container restart ends the follow stream cleanly; reopening follows the new container
// with the tail honoured, and the old stream never delivers another line.
func TestLogsFollowAcrossRestart(t *testing.T) {
	e := boot(t, nil)
	mark := e.app.Events.Mark()
	follow := func(a *kubernetes.LogsOpenArgs) { a.Follow = true }
	if err := e.open("w1", "f1", follow); err != nil {
		t.Fatal(err)
	}
	waitLines(t, e.app, mark, "w1", "f1", 3)
	e.fake.AppendLog("web-1", "live a")
	e.fake.AppendLog("web-1", "live b")
	first := waitLines(t, e.app, mark, "w1", "f1", 5)
	if want := []string{"line one", "line two", "line three", "live a", "live b"}; !slices.Equal(texts(first), want) {
		t.Fatalf("first run = %v", texts(first))
	}

	var boot []string
	for i := 1; i <= 103; i++ {
		boot = append(boot, fmt.Sprintf("boot %d", i))
	}
	e.fake.RestartContainer("web-1", boot...)
	lines, errMsg := waitEnded(t, e.app, mark, "w1", "f1")
	if errMsg != "" || !slices.Equal(texts(lines), texts(first)) {
		t.Fatalf("stream after restart = %v err %q, want it to end cleanly with no new lines", texts(lines), errMsg)
	}

	// Reconnect with the default tail of 100: the newest 100 lines of the new container, in order.
	if err := e.open("w1", "f2", follow); err != nil {
		t.Fatal(err)
	}
	if got := waitLines(t, e.app, mark, "w1", "f2", 100); !slices.Equal(texts(got), boot[3:]) {
		t.Fatalf("reconnect lines = %d from %q, want the last 100 of the new container", len(got), texts(got)[0])
	}
	e.fake.AppendLog("web-1", "boot 104")
	got := waitLines(t, e.app, mark, "w1", "f2", 101)
	if !slices.Equal(texts(got), append(boot[3:], "boot 104")) {
		t.Fatalf("followed lines = %v, want each line once and in order", texts(got))
	}
	if old, _, _ := logsOf(e.app, mark, "w1", "f1"); len(old) != len(first) {
		t.Fatalf("the ended stream delivered %d more lines", len(old)-len(first))
	}
	e.contract(t, "event:kira:kubernetes:logs:restart", kubernetes.LogsEvent{StreamID: "f2", Lines: got[98:]})
}
