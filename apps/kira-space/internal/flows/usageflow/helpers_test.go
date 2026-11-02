package usageflow_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentusage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const waitFor = 20 * time.Second

var settingsArg = regexp.MustCompile(`--settings '([^']+)'`)

// launch composes a claude launch the way Terminal.Open does and returns the settings document and
// the env the shell would see.
func launch(t *testing.T, app *flowharness.App, terminalID, cwd string) (doc map[string]any, settingsPath string, env []string) {
	t.Helper()
	command, env, _ := app.W.AgentHooks.Compose("claude", terminalID, cwd, "claude", "")
	m := settingsArg.FindStringSubmatch(command)
	if m == nil || len(env) == 0 {
		t.Fatalf("no --settings or env in %q", command)
	}
	raw, err := os.ReadFile(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc, m[1], env
}

// statusLineCommand is the statusLine.command string of a settings document ("" when absent).
func statusLineCommand(doc map[string]any) string {
	sl, _ := doc["statusLine"].(map[string]any)
	c, _ := sl["command"].(string)
	return c
}

// runStatusLine runs the settings document's statusLine command with payload on stdin, as Claude
// Code does, and returns its stdout.
func runStatusLine(t *testing.T, doc map[string]any, env []string, payload string) string {
	t.Helper()
	cmdText := statusLineCommand(doc)
	if cmdText == "" {
		t.Fatalf("no statusLine in %v", doc)
	}
	cmd := exec.Command("sh", "-c", cmdText)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("statusLine command: %v", err)
	}
	return string(out)
}

// payload is a statusline document; five and seven are used_percentage, resets are epoch seconds.
func payload(five float64, fiveReset int64, seven float64, sevenReset int64) string {
	return fmt.Sprintf(`{"model":{"id":"x"},"rate_limits":{"five_hour":{"used_percentage":%v,"resets_at":%d},"seven_day":{"used_percentage":%v,"resets_at":%d}}}`,
		five, fiveReset, seven, sevenReset)
}

func in(d time.Duration) int64 { return time.Now().Add(d).Unix() }

func usage(t *testing.T, app *flowharness.App) agentusage.Snapshot {
	t.Helper()
	snap, err := app.W.AgentUsageSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func waitState(t *testing.T, app *flowharness.App, pred func(agentusage.Snapshot) bool) agentusage.Snapshot {
	t.Helper()
	var last agentusage.Snapshot
	testx.WaitUntil(t, waitFor, func() bool {
		last = usage(t, app)
		return pred(last)
	})
	return last
}

func setUsage(t *testing.T, app *flowharness.App, on bool) {
	t.Helper()
	p := model.AgentPatch{UsageEnabled: &on}
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Agent: &p}}); err != nil {
		t.Fatalf("Settings.Set: %v", err)
	}
}

// newApp boots the app with CLAUDE_CONFIG_DIR cleared so the user's own statusline comes from the
// isolated HOME.
func newApp(t *testing.T, opts ...flowharness.Opt) *flowharness.App {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return flowharness.New(t, opts...)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string { return fmt.Sprint(n) }
