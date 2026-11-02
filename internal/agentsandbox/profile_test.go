package agentsandbox

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/harness/claude"
)

var update = flag.Bool("update", false, "rewrite golden profiles")

// testRuntime is a Claude runtime on home with no resolved binary.
func testRuntime(home string) Runtime {
	p := claude.New()
	return Runtime{Home: home, Agent: p.SandboxNeeds(home, ""), Provider: p}
}

func fakeRuntime() Runtime {
	p := claude.New()
	return Runtime{
		Home:         "/Users/ada",
		Agent:        p.SandboxNeeds("/Users/ada", "/Users/ada/.local/share/claude/bin/claude"),
		Provider:     p,
		PathPrefixes: []string{"/Users/ada/.nvm/versions/node/v22.1.0", "/Users/ada/go"},
		UserTemp:     "/private/var/folders/ab/cd/T",
		UserCache:    "/private/var/folders/ab/cd/C",
		SSHAuthSock:  "/private/tmp/com.apple.launchd.X/Listeners",
	}
}

const (
	scratch = "/Users/ada/Library/Application Support/Kira/agent/sandbox/run-1"
	cache   = "/Users/ada/Library/Application Support/Kira/agent/sandbox-cache"

	configDir = "/Users/ada/Library/Application Support/Kira/agent"
)

func linked(wt, name string) GitLayout {
	return GitLayout{Worktree: wt, GitDir: "/Users/ada/src/app/.git/worktrees/" + name, CommonDir: "/Users/ada/src/app/.git"}
}

func basePolicy() Policy {
	wt := "/Users/ada/src/app-wt/feat"
	return Policy{
		RunID: "run-1", Access: AccessWrite, Write: []string{wt}, RunDir: wt, Network: true,
		Git: []GitLayout{linked(wt, "feat")}, Scratch: scratch, Cache: cache, ConfigDir: configDir, Ports: []int{4100},
		Branch: map[string]string{wt: "task/feat"},
	}
}

func TestProfileGolden(t *testing.T) {
	rt := fakeRuntime()
	cases := map[string]func() (Policy, Runtime){
		"write_linked": func() (Policy, Runtime) { return basePolicy(), rt },
		"write_main": func() (Policy, Runtime) {
			p := basePolicy()
			wt := "/Users/ada/src/app"
			p.Write, p.RunDir = []string{wt}, wt
			p.Git = []GitLayout{{Worktree: wt, GitDir: wt + "/.git", CommonDir: wt + "/.git"}}
			p.Branch = map[string]string{wt: "main"}
			return p, rt
		},
		"read_folders": func() (Policy, Runtime) {
			p := basePolicy()
			wt := p.Write[0]
			p.Access, p.Write = AccessRead, nil
			p.Read = []string{wt, "/Users/ada/specs/payments"}
			p.Allow = []string{"/Users/ada/.cache/my-tool"}
			return p, rt
		},
		"network_off": func() (Policy, Runtime) {
			p := basePolicy()
			p.Network, p.Ports = false, []int{4100, 4200}
			return p, rt
		},
		"rebase_stack": func() (Policy, Runtime) {
			p := basePolicy()
			a, b := p.Write[0], "/Users/ada/src/app-wt/feat2"
			p.Write = []string{a, b}
			p.Git = []GitLayout{linked(a, "feat"), linked(b, "feat2")}
			p.Branch = map[string]string{a: "task/feat", b: "task/feat2"}
			return p, rt
		},
		"smart_run_dir": func() (Policy, Runtime) {
			p := basePolicy()
			dir, wt := "/Users/ada/scripts/nightly", p.Write[0]
			p.Write, p.RunDir, p.Read = []string{dir}, dir, []string{wt}
			p.Git = []GitLayout{linked(wt, "feat")}
			p.Branch = nil
			return p, rt
		},
		"allow_over_common": func() (Policy, Runtime) {
			p := basePolicy()
			p.Allow = []string{"/Users/ada/src/app/.git", "/Users/ada/.cache/my-tool", "/Users/ada/.claude/hooks"}
			return p, rt
		},
		"path_escaping": func() (Policy, Runtime) {
			p := basePolicy()
			p.Allow = []string{`/Users/ada/we"ird\dir`, "/Users/ada/sp ace"}
			p.Read = []string{"/Users/ada/dots.and+plus"}
			return p, rt
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			p, r := mk()
			got := Profile(p, r)
			path := filepath.Join("testdata", name+".sb")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("profile drifted from %s; run go test ./internal/agentsandbox -update and review the diff\n%s", path, got)
			}
		})
	}
}

// Every --mcp-config file a run gets must be readable inside its sandbox.
func TestProfileAllowsMCPConfigs(t *testing.T) {
	p := basePolicy()
	prof := Profile(p, fakeRuntime())
	_, allowRead, _ := strings.Cut(prof, "(allow file-read-data")
	allowRead, _, _ = strings.Cut(allowRead, "))\n")
	allowRead += ")"
	var res []*regexp.Regexp
	for _, m := range regexp.MustCompile(`\(regex #"([^"]+)"\)`).FindAllStringSubmatch(allowRead, -1) {
		res = append(res, regexp.MustCompile(m[1]))
	}
	for _, name := range []string{"run-1-7.mcp.json", "run-1-app.mcp.json", "run-1-app.settings.json", "run-1-app.merged.json"} {
		path := p.ConfigDir + "/" + name
		ok := false
		for _, re := range res {
			ok = ok || re.MatchString(path)
		}
		if !ok {
			t.Errorf("no read allow for %s", path)
		}
	}
	for _, name := range []string{"run-2-7.mcp.json", "run-1-7.mcp.json.bak", "run-1-x.mcp.json", "run-1-user.mcp.json", "run-2-app.mcp.json", "run-1-app.mcp.json.bak"} {
		for _, re := range res {
			if re.MatchString(p.ConfigDir + "/" + name) {
				t.Errorf("read allow too wide: %s", name)
			}
		}
	}
}
