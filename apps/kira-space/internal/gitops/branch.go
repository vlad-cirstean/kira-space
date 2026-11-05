package gitops

import (
	"regexp"
	"strings"
)

// BranchCreateArgs builds `git branch <name> <startPoint>`. An explicit upstream goes through
// BranchSetUpstreamArgs: `git branch` has no `-t <upstream>` form, `-t` is a bare flag.
func BranchCreateArgs(name, startPoint string) []string {
	return []string{"branch", name, startPoint}
}

// RecreateRefArgs builds `git update-ref <ref> <sha> ""`: the empty old value makes git refuse
// when the ref exists again, so an undo never moves a ref recreated outside the app.
func RecreateRefArgs(ref, sha string) []string {
	return []string{"update-ref", ref, sha, ""}
}

// BranchCreateAndSwitchArgs builds `git switch -c <name> <startPoint>`.
func BranchCreateAndSwitchArgs(name, startPoint string) []string {
	return []string{"switch", "-c", name, startPoint}
}

// BranchSetUpstreamArgs builds `git branch --set-upstream-to=<upstream> <name>` — the second argv
// of a branchCreate request whose explicit track differs from plain DWIM-on-startPoint.
func BranchSetUpstreamArgs(name, upstream string) []string {
	return []string{"branch", "--set-upstream-to=" + upstream, name}
}

// BranchDeleteArgs builds `git branch -d/-D <name>`.
func BranchDeleteArgs(name string, force bool) []string {
	if force {
		return []string{"branch", "-D", name}
	}
	return []string{"branch", "-d", name}
}

// BranchRenameArgs builds `git branch -m <from> <to>`.
func BranchRenameArgs(from, to string) []string {
	return []string{"branch", "-m", from, to}
}

// BranchUnsetUpstreamArgs builds `git branch --unset-upstream <name>`.
func BranchUnsetUpstreamArgs(name string) []string {
	return []string{"branch", "--unset-upstream", name}
}

// BranchRevParseArgs is undo-capture read #1: does the branch even still resolve, immediately
// before a delete — this is what makes the recovery sha the one that existed right before the
// delete, not a stale guess.
func BranchRevParseArgs(name string) []string {
	return []string{"rev-parse", "--verify", "refs/heads/" + name}
}

// BranchConfigRegexpArgs is undo-capture read: every local branch.<name>.* config entry, NUL-framed
// (`key\nvalue\0`, so a multi-line value never splits into fake entries) —
// replayed back through `git config` on undo so .remote/.merge (and anything else set under that
// prefix) comes back exactly.
//
// This file used to splice name into the
// pattern unescaped, on the premise that "a branch name containing regex metacharacters only
// widens the match, never narrows it to miss the real one" — true about a false NEGATIVE, but
// exactly backwards about the real risk, a false POSITIVE. A legal git branch name may contain
// POSIX ERE metacharacters (`|` in particular: git's own ref-name rules forbid space, ~, ^, :, ?,
// *, [, \, "..", "@{", but not "|"), and ERE's own precedence rules give `|` the LOWEST binding —
// a branch named "x|y" turns `^branch\.x|y\.` into "starts with branch.x, OR contains y." ANYWHERE
// in the key — capturing config lines for unrelated branches, or unrelated sections entirely, that
// this delete never touched. Those over-captured lines are then queued into the undo record's own
// Replay list (captureBranchDeleteUndo, ops.go) as unconditional `git config <key> <value>` SETs —
// so undoing THIS branch's delete could silently overwrite some other, wholly unrelated config key
// back to a stale captured value. regexp.QuoteMeta escapes exactly POSIX ERE's own metacharacter
// set (`\.+*?()|[]{}^$`), so the escaped name can only ever match itself, literally.
func BranchConfigRegexpArgs(name string) []string {
	return []string{"config", "--local", "--null", "--get-regexp", `^branch\.` + regexp.QuoteMeta(name) + `\.`}
}

// BranchConfigRestoreArgs parses BranchConfigRegexpArgs output into one `config --add` argv per
// entry. --add (not a plain set) because the section is gone after the delete and a multi-valued
// key must come back with every value; `--` keeps a value starting with `-` from parsing as a flag.
// A valueless boolean key (no newline in its record) is restored as "true".
func BranchConfigRestoreArgs(raw []byte) [][]string {
	var argvList [][]string
	for _, rec := range strings.Split(string(raw), "\x00") {
		if rec == "" {
			continue
		}
		key, value, found := strings.Cut(rec, "\n")
		if !found {
			value = "true"
		}
		argvList = append(argvList, []string{"config", "--local", "--add", "--", key, value})
	}
	return argvList
}
