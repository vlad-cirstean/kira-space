package dockerflow

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const wait = 20 * time.Second

// boot returns a booted app and a client to the real engine; skips (or fails under
// KIRA_FLOW_DOCKER=require) without one.
func boot(t *testing.T) (*flowharness.App, *flowharness.Docker) {
	t.Helper()
	d := flowharness.RequireDocker(t)
	return flowharness.New(t), d
}

// sleeper starts a labelled container that prints "ready" and sleeps; stops within a second.
func sleeper(d *flowharness.Docker, name string, labels map[string]string) string {
	return shellBox(d, name, "echo ready; exec sleep 300", labels)
}

func shellBox(d *flowharness.Docker, name, script string, labels map[string]string) string {
	stop := 1
	return d.RunWith(name, container.Config{Cmd: []string{"sh", "-c", script}, StopTimeout: &stop}, nil, labels)
}

func findContainer(list []docker.Container, id string) *docker.Container {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

// logsOf returns the lines and end state of one logs stream recorded since mark.
func logsOf(app *flowharness.App, mark int, window, streamID string) (lines []docker.LogLine, ended bool, errMsg string) {
	for _, ev := range app.Events.Since(mark, docker.ChannelLogs) {
		le, ok := ev.Data.(docker.LogsEvent)
		if !ok || le.StreamID != streamID || ev.Window != window {
			continue
		}
		lines = append(lines, le.Lines...)
		if le.Ended {
			ended, errMsg = true, le.Error
		}
	}
	return lines, ended, errMsg
}

func waitLines(t *testing.T, app *flowharness.App, mark int, window, streamID string, n int) []docker.LogLine {
	t.Helper()
	var lines []docker.LogLine
	testx.WaitUntil(t, wait, func() bool {
		var ended bool
		lines, ended, _ = logsOf(app, mark, window, streamID)
		return len(lines) >= n || ended
	})
	return lines
}

func waitLogsEnded(t *testing.T, app *flowharness.App, mark int, window, streamID string) []docker.LogLine {
	t.Helper()
	var lines []docker.LogLine
	testx.WaitUntil(t, wait, func() bool {
		var ended bool
		lines, ended, _ = logsOf(app, mark, window, streamID)
		return ended
	})
	return lines
}

// execOut returns the decoded output and exit of one exec session recorded since mark.
func execOut(app *flowharness.App, mark int, window, id string) (out string, exited bool, code int) {
	var sb strings.Builder
	for _, ev := range app.Events.Since(mark, docker.ChannelExec) {
		te, ok := ev.Data.(terminal.Event)
		if !ok || te.TerminalID != id || ev.Window != window {
			continue
		}
		if te.Data != "" {
			raw, _ := base64.StdEncoding.DecodeString(te.Data)
			sb.Write(raw)
		}
		if te.Exited {
			exited = true
			if te.ExitCode != nil {
				code = *te.ExitCode
			}
		}
	}
	return sb.String(), exited, code
}

func write(t *testing.T, app *flowharness.App, id, text string) {
	t.Helper()
	if err := app.W.Docker.ExecWrite(docker.ExecWriteArgs{TerminalID: id, Data: base64.StdEncoding.EncodeToString([]byte(text))}); err != nil {
		t.Fatal(err)
	}
}

func statsFor(app *flowharness.App, mark int, window, id string) []docker.StatsSample {
	var out []docker.StatsSample
	for _, ev := range app.Events.Since(mark, docker.ChannelStats) {
		se, ok := ev.Data.(docker.StatsEvent)
		if !ok || ev.Window != window {
			continue
		}
		for _, s := range se.Samples {
			if s.ID == id {
				out = append(out, s)
			}
		}
	}
	return out
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// waitUntil polls cond, bounded, for a daemon-side state change (e.g. a stopped container
// dropping out of an all:false list) that a shared CI docker daemon under concurrent load from
// every other adapter test's own containers can lag a beat behind the client call that caused it.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
