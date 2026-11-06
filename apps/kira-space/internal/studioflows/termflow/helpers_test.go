package termflow

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const wait = 20 * time.Second

func open(t *testing.T, app *studioflowharness.App, window, id, cwd string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: id, WindowKey: window, Cwd: cwd, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
}

// out returns the decoded output and exit of one terminal recorded since mark, for window.
func out(app *studioflowharness.App, mark int, window, id string) (text string, exited bool, code int) {
	var sb strings.Builder
	for _, ev := range app.Events.Since(mark, appevent.ChannelTerminal) {
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

// waitMatch waits until the terminal's output matches re and returns the first submatch set.
func waitMatch(t *testing.T, app *studioflowharness.App, mark int, window, id string, re *regexp.Regexp) []string {
	t.Helper()
	var m []string
	testx.WaitUntil(t, wait, func() bool {
		text, _, _ := out(app, mark, window, id)
		m = re.FindStringSubmatch(text)
		return m != nil
	})
	return m
}

func waitExit(t *testing.T, app *studioflowharness.App, mark int, window, id string) int {
	t.Helper()
	var code int
	testx.WaitUntil(t, wait, func() bool {
		var exited bool
		_, exited, code = out(app, mark, window, id)
		return exited
	})
	return code
}

func write(t *testing.T, app *studioflowharness.App, id, text string) {
	t.Helper()
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: id, Data: base64.StdEncoding.EncodeToString([]byte(text))}); err != nil {
		t.Fatal(err)
	}
}

// line matches a whole output line (a pty may lead it with a bare CR), so the typed command's echo does not count.
func line(s string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)(?:^|\r)` + regexp.QuoteMeta(s) + `\r?$`)
}
