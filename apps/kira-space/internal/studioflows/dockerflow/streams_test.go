package dockerflow

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// settle is the quiet period a "nothing more arrives" assertion waits out.
const settle = 600 * time.Millisecond

func TestEngineEvents(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker
	if err := svc.Watch(docker.WindowArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	// The engine stream attaches asynchronously; retry until a container start is seen.
	seen := false
	for i := 0; i < 10 && !seen; i++ {
		mark := app.Events.Mark()
		sleeper(d, fmt.Sprintf("ev%d", i), nil)
		deadline := time.Now().Add(3 * time.Second)
		for !seen && time.Now().Before(deadline) {
			for _, ev := range app.Events.Since(mark, docker.ChannelChanged) {
				var ce docker.ChangedEvent
				ev.Decode(t, &ce)
				seen = seen || contains(ce.Kinds, "container")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !seen {
		t.Fatal("no kira:docker:changed naming containers")
	}

	if err := svc.Unwatch(docker.WindowArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(settle)
	mark := app.Events.Mark()
	sleeper(d, "after-unwatch", nil)
	time.Sleep(settle)
	if got := app.Events.Since(mark, docker.ChannelChanged); len(got) != 0 {
		t.Fatalf("%d changed events after Unwatch", len(got))
	}
}

func TestLogsStream(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker

	id := shellBox(d, "logs", `i=1; while [ $i -le 50 ]; do echo out$i; echo err$i >&2; i=$((i+1)); done; exec sleep 300`, nil)
	mark := app.Events.Mark()
	if err := svc.LogsOpen(docker.LogsOpenArgs{WindowKey: "w1", StreamID: "s1", ContainerID: id, Tail: -1, Follow: true, Timestamps: true}); err != nil {
		t.Fatal(err)
	}
	lines := waitLines(t, app, mark, "w1", "s1", 100)
	var outs, errs []string
	for _, l := range lines {
		if l.TS == "" {
			t.Fatalf("line %+v has no timestamp", l)
		}
		switch l.Stream {
		case "stdout":
			outs = append(outs, l.Text)
		case "stderr":
			errs = append(errs, l.Text)
		}
	}
	for i := 1; i <= 50; i++ {
		if len(outs) < i || outs[i-1] != fmt.Sprintf("out%d", i) || len(errs) < i || errs[i-1] != fmt.Sprintf("err%d", i) {
			t.Fatalf("line %d out of order or missing: stdout=%d stderr=%d", i, len(outs), len(errs))
		}
	}
	if len(lines) != 100 {
		t.Fatalf("got %d lines, want 100", len(lines))
	}

	if err := svc.LogsClose(docker.StreamArgs{StreamID: "s1"}); err != nil {
		t.Fatal(err)
	}
	waitLogsEnded(t, app, mark, "w1", "s1")
	after := app.Events.Mark()
	time.Sleep(settle)
	if n := len(app.Events.Since(after, docker.ChannelLogs)); n != 0 {
		t.Fatalf("%d logs events after LogsClose ended the stream", n)
	}

	// A container that exits ends its own stream.
	short := shellBox(d, "short", "echo a; sleep 1; echo b", nil)
	mark = app.Events.Mark()
	if err := svc.LogsOpen(docker.LogsOpenArgs{WindowKey: "w1", StreamID: "s2", ContainerID: short, Tail: -1, Follow: true}); err != nil {
		t.Fatal(err)
	}
	got := waitLogsEnded(t, app, mark, "w1", "s2")
	if len(got) != 2 || got[0].Text != "a" || got[1].Text != "b" {
		t.Fatalf("lines = %+v, want a then b", got)
	}

	err := svc.LogsOpen(docker.LogsOpenArgs{WindowKey: "w1", StreamID: "s3", ContainerID: "no-such-container", Tail: -1})
	if err == nil || testx.AsIpcErr(t, err).Code != "E_NOT_FOUND" {
		t.Fatalf("unknown container = %v, want E_NOT_FOUND", err)
	}
	if err := svc.LogsOpen(docker.LogsOpenArgs{WindowKey: "w1", StreamID: "s3", ContainerID: short, Tail: -1}); err != nil {
		t.Fatalf("stream id not released after a failed open: %v", err)
	}

	t.Run("100000 lines", func(t *testing.T) {
		studioflowharness.Complete(t)
		bulk := shellBox(d, "bulk", "seq 1 100000", nil)
		mark := app.Events.Mark()
		if err := svc.LogsOpen(docker.LogsOpenArgs{WindowKey: "w1", StreamID: "bulk", ContainerID: bulk, Tail: -1, Follow: true}); err != nil {
			t.Fatal(err)
		}
		lines := waitLogsEnded(t, app, mark, "w1", "bulk")
		if len(lines) != 100000 {
			t.Fatalf("got %d lines, want 100000", len(lines))
		}
		for i, l := range lines {
			if l.Text != fmt.Sprint(i+1) {
				t.Fatalf("line %d = %q", i, l.Text)
			}
		}
	})
}

func TestExecSession(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker
	id := sleeper(d, "exec", nil)

	mark := app.Events.Mark()
	open := docker.ExecOpenArgs{WindowKey: "w1", TerminalID: "x1", ContainerID: id, Cols: 80, Rows: 24}
	opened, err := svc.ExecOpen(open)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "docker-exec", "args:DockerService.ExecOpen", open, studioflowharness.Mask("containerId"))
	app.Contract(t, "docker-exec", "DockerService.ExecOpen", opened)
	write(t, app, "x1", "echo $((6*7))\n")
	answer := regexp.MustCompile(`(?m)^42\r?$`)
	testx.WaitUntil(t, wait, func() bool {
		out, _, _ := execOut(app, mark, "w1", "x1")
		return answer.MatchString(out)
	})

	if err := svc.ExecResize(docker.ExecResizeArgs{TerminalID: "x1", Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	write(t, app, "x1", "stty size\n")
	testx.WaitUntil(t, wait, func() bool {
		out, _, _ := execOut(app, mark, "w1", "x1")
		return regexp.MustCompile(`(?m)^30 100\r?$`).MatchString(out)
	})

	write(t, app, "x1", "exit 3\n")
	testx.WaitUntil(t, wait, func() bool {
		_, exited, _ := execOut(app, mark, "w1", "x1")
		return exited
	})
	if _, _, code := execOut(app, mark, "w1", "x1"); code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if err := svc.ExecClose(docker.ExecCloseArgs{TerminalID: "x1"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecClose(docker.ExecCloseArgs{TerminalID: "x1"}); err != nil {
		t.Fatalf("second ExecClose: %v", err)
	}

	if err := svc.Stop(docker.IDArgs{ID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecOpen(docker.ExecOpenArgs{WindowKey: "w1", TerminalID: "x2", ContainerID: id, Cols: 80, Rows: 24}); err == nil {
		t.Fatal("ExecOpen on a stopped container succeeded")
	}
}

func TestStatsTwoWindows(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker
	id := sleeper(d, "stats", nil)

	for _, w := range []string{"A", "B"} {
		if err := svc.StatsSubscribe(docker.StatsArgs{WindowKey: w, IDs: []string{id}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range []string{"A", "B"} {
		testx.WaitUntil(t, wait, func() bool {
			for _, s := range statsFor(app, 0, w, id) {
				if s.MemUsage > 0 && s.MemLimit > 0 && s.At > 0 {
					return true
				}
			}
			return false
		})
	}

	if err := svc.StatsUnsubscribe(docker.WindowArgs{WindowKey: "A"}); err != nil {
		t.Fatal(err)
	}
	// An emit already in flight may still land; the quiet check starts after B proves a full tick.
	mark := app.Events.Mark()
	testx.WaitUntil(t, wait, func() bool { return len(statsFor(app, mark, "B", id)) > 0 })
	mark = app.Events.Mark()
	testx.WaitUntil(t, wait, func() bool { return len(statsFor(app, mark, "B", id)) > 0 })
	if got := statsFor(app, mark, "A", id); len(got) != 0 {
		t.Fatalf("window A got %d samples after unsubscribing", len(got))
	}
}

func TestWindowCloseEndsStreams(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker
	ticker := shellBox(d, "ticker", "while :; do echo tick; sleep 0.2; done", nil)

	mark := app.Events.Mark()
	for _, a := range []docker.LogsOpenArgs{
		{WindowKey: "A", StreamID: "la", ContainerID: ticker, Tail: 0, Follow: true},
		{WindowKey: "B", StreamID: "lb", ContainerID: ticker, Tail: 0, Follow: true},
	} {
		if err := svc.LogsOpen(a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ExecOpen(docker.ExecOpenArgs{WindowKey: "A", TerminalID: "xa", ContainerID: ticker, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if err := svc.StatsSubscribe(docker.StatsArgs{WindowKey: "A", IDs: []string{ticker}}); err != nil {
		t.Fatal(err)
	}
	waitLines(t, app, mark, "A", "la", 1)
	waitLines(t, app, mark, "B", "lb", 1)
	testx.WaitUntil(t, wait, func() bool { return len(statsFor(app, mark, "A", ticker)) > 0 })

	app.CloseWindow("A")
	waitLogsEnded(t, app, mark, "A", "la")
	testx.WaitUntil(t, wait, func() bool {
		_, exited, _ := execOut(app, mark, "A", "xa")
		return exited
	})
	time.Sleep(settle)
	after := app.Events.Mark()
	testx.WaitUntil(t, wait, func() bool {
		lines, _, _ := logsOf(app, after, "B", "lb")
		return len(lines) > 0
	})
	if n := len(statsFor(app, after, "A", ticker)); n != 0 {
		t.Fatalf("window A still gets stats: %d samples", n)
	}
	if _, ended, _ := logsOf(app, mark, "B", "lb"); ended {
		t.Fatal("window B's logs ended with window A")
	}
}

func TestContexts(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker

	meta := fmt.Sprintf(`{"Name":"flow-ctx","Metadata":{"Description":"flow"},"Endpoints":{"docker":{"Host":%q,"SkipTLSVerify":false}}}`, d.Host)
	dir := app.Home + "/.docker/contexts/meta/" + sha256Hex("flow-ctx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/meta.json", []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}

	ctxs, err := svc.Contexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(ctxs) != 2 || ctxs[0].Name != "default" || ctxs[1].Name != "flow-ctx" || ctxs[1].Host != d.Host {
		t.Fatalf("contexts = %+v, want default then flow-ctx", ctxs)
	}

	id := shellBox(d, "ctx", "while :; do echo tick; sleep 0.2; done", nil)
	mark := app.Events.Mark()
	if err := svc.LogsOpen(docker.LogsOpenArgs{WindowKey: "w1", StreamID: "s1", ContainerID: id, Tail: 0, Follow: true}); err != nil {
		t.Fatal(err)
	}
	waitLines(t, app, mark, "w1", "s1", 1)

	mark = app.Events.Mark()
	st, err := svc.UseContext(docker.UseContextArgs{Name: "flow-ctx"})
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "ok" || st.Endpoint.Context != "flow-ctx" || st.Endpoint.Source != "selected" {
		t.Fatalf("status = %+v, want ok via selected flow-ctx", st)
	}
	app.Events.WaitAfter(t, mark, docker.ChannelStatus, nil, wait)
	app.Events.WaitAfter(t, mark, docker.ChannelChanged, nil, wait)
	waitLogsEnded(t, app, mark, "w1", "s1")
	if after, _ := svc.Contexts(); !after[1].Current {
		t.Fatalf("flow-ctx not current: %+v", after)
	}

	_, err = svc.UseContext(docker.UseContextArgs{Name: "bogus"})
	if err == nil || testx.AsIpcErr(t, err).Code != "E_BAD_REQUEST" {
		t.Fatalf("bogus context = %v, want E_BAD_REQUEST", err)
	}
	st, err = svc.UseContext(docker.UseContextArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "ok" || st.Endpoint.Context != "default" || st.Endpoint.Source == "selected" {
		t.Fatalf("status = %+v, want ok via automatic resolution", st)
	}
}

func TestQuitEndsStreams(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker
	id := shellBox(d, "quit", "while :; do echo tick; sleep 0.2; done", nil)

	mark := app.Events.Mark()
	if err := svc.LogsOpen(docker.LogsOpenArgs{WindowKey: "w1", StreamID: "s1", ContainerID: id, Tail: 0, Follow: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecOpen(docker.ExecOpenArgs{WindowKey: "w1", TerminalID: "x1", ContainerID: id, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	waitLines(t, app, mark, "w1", "s1", 1)

	app.Quit(t)
	waitLogsEnded(t, app, mark, "w1", "s1")
	testx.WaitUntil(t, wait, func() bool {
		_, exited, _ := execOut(app, mark, "w1", "x1")
		return exited
	})

	fresh := studioflowharness.New(t)
	list, err := fresh.W.Docker.Containers(docker.ListArgs{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if findContainer(list, id) == nil {
		t.Fatal("fresh app does not list the container")
	}
}

func TestComposeGrouping(t *testing.T) {
	studioflowharness.Complete(t)
	app, d := boot(t)
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("docker compose is not installed")
	}
	project := "kiraflow" + uuid.NewString()[:8]
	compose := func(args ...string) {
		t.Helper()
		cmd := exec.Command("docker", append([]string{"compose", "-f", "testdata/compose.yml", "-p", project}, args...)...)
		cmd.Env = append(os.Environ(), "DOCKER_HOST="+d.Host, "KIRA_FLOWTEST_RUN="+uuid.NewString()[:8])
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("docker compose %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	t.Cleanup(func() {
		cmd := exec.Command("docker", "compose", "-f", "testdata/compose.yml", "-p", project, "down", "-v", "-t", "1")
		cmd.Env = append(os.Environ(), "DOCKER_HOST="+d.Host, "KIRA_FLOWTEST_RUN=x")
		_ = cmd.Run()
	})
	compose("up", "-d")

	list, err := app.W.Docker.Containers(docker.ListArgs{All: true})
	if err != nil {
		t.Fatal(err)
	}
	services := map[string]bool{}
	for _, c := range list {
		if c.ComposeProject == project {
			services[c.ComposeService] = true
		}
	}
	if len(services) != 2 || !services["web"] || !services["cache"] {
		t.Fatalf("project %s services = %v, want web and cache", project, services)
	}
}
