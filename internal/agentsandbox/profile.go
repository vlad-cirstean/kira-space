package agentsandbox

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
)

var shellRC = []string{".zshenv", ".zprofile", ".zshrc", ".zlogin", ".bashrc", ".bash_profile", ".profile"}

// Per-worktree state files inside a main worktree's .git that commit, merge and rebase write.
var mainStateFiles = []string{
	"index", "index.lock", "HEAD", "HEAD.lock", "ORIG_HEAD", "ORIG_HEAD.lock", "AUTO_MERGE",
	"MERGE_HEAD", "MERGE_MSG", "MERGE_MODE", "CHERRY_PICK_HEAD", "REVERT_HEAD", "COMMIT_EDITMSG",
	"logs/HEAD", "logs/HEAD.lock",
}

var mainStateDirs = []string{"rebase-merge", "rebase-apply", "sequencer"}

// Profile renders the Seatbelt profile for p. Later rules win, so the sections run from broad
// allows to specific denies.
func Profile(p Policy, rt Runtime) string {
	b := &sbpl{}
	b.line("(version 1)")
	b.line(fmt.Sprintf(";; %s run=%s access=%s network=%t", GeneratorVersion, p.RunID, p.Access, p.Network))
	if len(p.Ports) > 0 {
		b.line(";; loopback ports: " + joinInts(p.Ports))
	}
	b.line("(allow default)")

	b.section("escapes")
	b.line("(deny appleevent-send)")
	b.rule("deny", "mach-lookup", gname("com.apple.coreservices.launchservicesd"), gprefix("com.apple.lsd."), gname("com.apple.pasteboard.1"))
	b.line("(deny signal)")
	b.line("(allow signal (target same-sandbox))")

	b.section("reads")
	denyRead := []string{sub("/Users"), sub("/Volumes")}
	if rt.Home != "" && !under(rt.Home, "/Users") && !under(rt.Home, "/Volumes") {
		denyRead = append(denyRead, sub(rt.Home))
	}
	b.rule("deny", "file-read-data", denyRead...)
	var allowRead []string
	if rt.Home != "" {
		h := rt.Home
		allowRead = append(allowRead, agentConfig(rt.Agent)...)
		allowRead = append(allowRead, lit(filepath.Join(h, ".gitconfig")), sub(filepath.Join(h, ".config", "git")))
		for _, f := range shellRC {
			allowRead = append(allowRead, lit(filepath.Join(h, f)))
		}
		allowRead = append(allowRead, sub(filepath.Join(h, ".kira-memory")), sub(filepath.Join(h, "Library", "Keychains")))
	}
	allowRead = append(allowRead, subs(rt.PathPrefixes)...)
	if rt.Agent.InstallPrefix != "" {
		allowRead = append(allowRead, sub(rt.Agent.InstallPrefix))
	}
	allowRead = append(allowRead, subs(p.Read)...)
	allowRead = append(allowRead, subs(p.Allow)...)
	allowRead = append(allowRead, subs(p.Write)...)
	for _, g := range p.Git {
		allowRead = append(allowRead, sub(g.GitDir))
		if g.Linked() {
			allowRead = append(allowRead, sub(g.CommonDir))
		}
	}
	allowRead = append(allowRead, sub(p.Scratch), sub(p.Cache))
	if p.ConfigDir != "" {
		allowRead = append(allowRead, rx("^"+regexp.QuoteMeta(p.ConfigDir)+"/"+regexp.QuoteMeta(p.RunID)+`-([0-9]+\.mcp|app\.[a-z]+)\.json$`))
	}
	if p.ContextDir != "" {
		allowRead = append(allowRead, sub(p.ContextDir))
	}
	for _, f := range p.ReadFiles {
		allowRead = append(allowRead, lit(f))
	}
	b.rule("allow", "file-read-data", dedupe(allowRead)...)

	b.section("writes")
	b.line("(deny file-write*)")
	allowWrite := []string{
		lit("/dev/null"), lit("/dev/zero"), lit("/dev/tty"), lit("/dev/dtracehelper"),
		rx(`^/dev/ttys[0-9]+$`), rx(`^/dev/fd/`), sub(p.Scratch), sub(p.Cache),
	}
	if rt.UserTemp != "" {
		allowWrite = append(allowWrite, sub(rt.UserTemp))
	}
	if rt.UserCache != "" {
		allowWrite = append(allowWrite, sub(rt.UserCache))
	}
	if rt.Home != "" {
		h := rt.Home
		allowWrite = append(allowWrite, agentConfig(rt.Agent)...)
		allowWrite = append(allowWrite, sub(filepath.Join(h, ".kira-memory")))
	}
	b.rule("allow", "file-write*", allowWrite...)
	if len(p.Allow) > 0 {
		b.section("allow paths")
		b.rule("allow", "file-write*", subs(p.Allow)...)
	}

	if p.Access == AccessWrite {
		b.section("worktrees")
		b.rule("allow", "file-write*", subs(p.Write)...)
		gitRules(b, p)
	}

	protectWrites(b, rt)

	b.section("network")
	if !p.Network {
		b.line("(deny network*)")
		b.line(`(allow network* (local ip "localhost:*") (remote ip "localhost:*"))`)
		b.line("(allow network-outbound (remote unix-socket))")
	}
	if rt.SSHAuthSock != "" {
		b.line(fmt.Sprintf("(deny network-outbound (remote unix-socket (path-literal %s)))", q(rt.SSHAuthSock)))
	}
	return b.String()
}

// agentConfig is the agent's config dirs, then its config files.
func agentConfig(n harness.SandboxNeeds) []string {
	out := make([]string, 0, len(n.ConfigDirs)+len(n.ConfigFiles))
	for _, d := range n.ConfigDirs {
		out = append(out, sub(d))
	}
	for _, f := range n.ConfigFiles {
		out = append(out, rx("^"+regexp.QuoteMeta(f)))
	}
	return out
}

// protectWrites denies the config, hook and keychain writes last, so no allow path reopens them.
func protectWrites(b *sbpl, rt Runtime) {
	if rt.Home == "" {
		return
	}
	n := rt.Agent
	protect := make([]string, 0, len(n.ProtectFiles)+len(n.ProtectDirs)+1)
	if len(n.ConfigDirs) > 0 {
		for _, f := range n.ProtectFiles {
			protect = append(protect, lit(filepath.Join(n.ConfigDirs[0], f)))
		}
		for _, d := range n.ProtectDirs {
			protect = append(protect, sub(filepath.Join(n.ConfigDirs[0], d)))
		}
	}
	protect = append(protect, sub(filepath.Join(rt.Home, "Library", "Keychains")))
	b.section("protections")
	b.rule("deny", "file-write*", protect...)
}

// gitRules emits the minimal git write set per worktree, protections last so no allow path reopens
// the common dir.
func gitRules(b *sbpl, p Policy) {
	var writable []GitLayout
	for _, g := range p.Git {
		if containsPath(p.Write, g.Worktree) {
			writable = append(writable, g)
		}
	}
	if len(writable) == 0 {
		return
	}
	b.section("git")
	var commons []string
	for _, g := range writable {
		commons = appendUnique(commons, g.CommonDir)
	}
	for _, c := range commons {
		b.rule("deny", "file-write*", sub(c))
	}
	for _, g := range writable {
		c := g.CommonDir
		if g.Linked() {
			b.rule("allow", "file-write*", sub(g.GitDir))
		} else {
			var state []string
			for _, f := range mainStateFiles {
				state = append(state, lit(filepath.Join(c, f)))
			}
			for _, d := range mainStateDirs {
				state = append(state, sub(filepath.Join(c, d)))
			}
			b.rule("allow", "file-write*", state...)
		}
		b.rule("allow", "file-write*", sub(filepath.Join(c, "objects")))
		if br := p.Branch[g.Worktree]; br != "" {
			ownRef(b, c, br)
		}
	}
	for _, g := range writable {
		if !g.Linked() {
			continue
		}
		b.rule("deny", "file-write*", lit(filepath.Join(g.Worktree, ".git")), lit(filepath.Join(g.GitDir, "gitdir")),
			lit(filepath.Join(g.GitDir, "commondir")), lit(filepath.Join(g.GitDir, "locked")), lit(filepath.Join(g.GitDir, "config.worktree")))
		b.rule("deny", "file-write-unlink", lit(g.GitDir))
	}
	for _, c := range commons {
		qc := regexp.QuoteMeta(c)
		b.rule("deny", "file-write-unlink file-write-data file-write-mode",
			rx("^"+qc+`/objects/[0-9a-f]{2}/[0-9a-f]{38,62}$`),
			rx("^"+qc+`/objects/pack/pack-[0-9a-f]+\.(pack|idx|rev|bitmap|keep)$`))
	}
}

// ownRef lets the run advance its own branch: the loose ref, its lock and reflog, and the
// directories a slashed name needs.
func ownRef(b *sbpl, common, branch string) {
	files := make([]string, 0, 4)
	var dirs []string
	for _, root := range []string{filepath.Join(common, "refs", "heads"), filepath.Join(common, "logs", "refs", "heads")} {
		ref := filepath.Join(root, filepath.FromSlash(branch))
		files = append(files, lit(ref), lit(ref+".lock"))
		for d := filepath.Dir(ref); d != root && strings.HasPrefix(d, root); d = filepath.Dir(d) {
			dirs = append(dirs, lit(d))
		}
	}
	b.rule("allow", "file-write*", files...)
	if len(dirs) > 0 {
		b.rule("allow", "file-write-create", dirs...)
	}
}

func containsPath(list []string, p string) bool {
	for _, l := range list {
		if l == p {
			return true
		}
	}
	return false
}

func under(p, root string) bool { return p == root || strings.HasPrefix(p, root+"/") }

func dedupe(list []string) []string { return appendUnique(nil, list...) }

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = fmt.Sprint(n)
	}
	return strings.Join(s, ",")
}

type sbpl struct{ strings.Builder }

func (b *sbpl) line(s string) { b.WriteString(s + "\n") }

func (b *sbpl) section(name string) { b.WriteString("\n;; " + name + "\n") }

func (b *sbpl) rule(verb, op string, filters ...string) {
	if len(filters) == 0 {
		return
	}
	b.WriteString("(" + verb + " " + op)
	for _, f := range filters {
		b.WriteString("\n  " + f)
	}
	b.WriteString(")\n")
}

// q renders an SBPL string literal; user text never reaches a regex.
func q(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func sub(p string) string   { return "(subpath " + q(p) + ")" }
func lit(p string) string   { return "(literal " + q(p) + ")" }
func rx(re string) string   { return `(regex #"` + re + `")` }
func gname(n string) string { return "(global-name " + q(n) + ")" }
func gprefix(n string) string {
	return "(global-name-prefix " + q(n) + ")"
}

func subs(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = sub(p)
	}
	return out
}
