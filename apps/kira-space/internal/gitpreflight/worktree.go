// The two worktree classifiers — pure functions over already-gathered facts, exactly like
// every other classifier in this package (ClassifyCheckout/ClassifyReset): no I/O here at all.
// gitsession/worktree.go gathers the facts (a fresh `worktree list`, a plain os.Stat,
// Registry.IsOpen) and calls these.
package gitpreflight

// WorktreeAddBlocker mirrors the wire's own WorktreeAddPreflight blocker union, flattened into one
// struct with omitempty per kind-specific field — CheckoutBlocker's own convention (D4).
type WorktreeAddBlocker struct {
	Kind         string `json:"kind"` // "invalidPath"|"pathExists"|"branchCheckedOutElsewhere"|"branchExists"|"unknownStartPoint"
	Path         string `json:"path,omitempty"`
	Branch       string `json:"branch,omitempty"`
	WorktreePath string `json:"worktreePath,omitempty"` // branchCheckedOutElsewhere
	StartPoint   string `json:"startPoint,omitempty"`
}

// WorktreeAddNote mirrors the wire's own WorktreeAddPreflight note union: informational only,
// never gates Verdict. pathInsideRepo is the named example (git allows nesting a worktree
// inside the repository it belongs to — refusing a legal action would be this app deciding for the
// user, not git); the other two are this implementer's own reasonable extension of the same
// "surface something true and possibly-surprising, block nothing" principle.
type WorktreeAddNote struct {
	Kind string `json:"kind"` // "pathInsideRepo"|"parentDirectoryMissing"|"detachedHead"
	Path string `json:"path,omitempty"`
}

// WorktreeAddPreflight mirrors the wire's own WorktreeAddPreflight field for field.
type WorktreeAddPreflight struct {
	Path     string               `json:"path"`
	Mode     string               `json:"mode"` // "existingBranch"|"newBranch"|"detach"
	Branch   string               `json:"branch,omitempty"`
	Blockers []WorktreeAddBlocker `json:"blockers"`
	Notes    []WorktreeAddNote    `json:"notes"`
	Verdict  string               `json:"verdict"` // "clean" | "blocked"
	// "detachHere" is offered iff branchCheckedOutElsewhere is the SOLE blocker — unlike
	// checkout's own automatic detach, this one is OFFERED via a dialog button, never taken
	// automatically, since `worktree add` is an explicit creation act with a dialog already open
	// and carries no "never blocks" promise to keep. Never nil (an empty slice, this package's own
	// convention elsewhere).
	Routes []string `json:"routes"`
}

// ClassifyWorktreeAddInput is ClassifyWorktreeAdd's own input — every fact gathered host-side
// before classification, none of it recomputed here.
type ClassifyWorktreeAddInput struct {
	Path       string
	Mode       string // "existingBranch" | "newBranch" | "detach"
	Branch     string // required for existingBranch/newBranch, "" for detach
	StartPoint string // required for newBranch/detach, "" for existingBranch (the branch name IS the commit-ish there)

	// PathExists: a plain os.Stat on Path succeeded — git itself refuses an existing non-empty
	// path (probe M1) and even an existing EMPTY directory in most configurations, so this blocks
	// unconditionally rather than trying to predict which case git would tolerate.
	PathExists bool
	// PathInsideRepo: Path is inside this repository's own working tree (D4's named note).
	PathInsideRepo bool
	// PathInsideGitDir: Path is inside the repository's own git directory, where a checkout would
	// corrupt the repository's metadata; blocks as invalidPath.
	PathInsideGitDir bool
	// ParentDirMissing: Path's immediate parent directory does not exist yet — git creates it
	// automatically, so this is a note, not a blocker.
	ParentDirMissing bool

	// BranchCheckedOutElsewhere: non-nil (the absolute worktree path) when Mode is
	// "existingBranch" and Branch is already checked out in a DIFFERENT worktree — read from the
	// same fresh refsSnapshot checkout pre-flight already uses (F2), no new spawn.
	BranchCheckedOutElsewhere *string
	// BranchExists: Mode is "newBranch" and Branch already names an existing ref — read from the
	// same refs snapshot, no new spawn.
	BranchExists bool

	// StartPointResolves: does the commit-ish that will be passed to `worktree add`
	// resolve to a real commit — StartPoint for newBranch/detach, Branch itself for
	// existingBranch. false only when the caller supplied garbage; the dialog always offers a
	// real branch/commit picker, so this is a defensive re-check, not the primary UX.
	StartPointResolves bool
}

// ClassifyWorktreeAdd is the classifier: five blockers in this exact order (invalidPath,
// pathExists, branchCheckedOutElsewhere, branchExists, unknownStartPoint — earlier blockers do not
// suppress later ones; every applicable blocker is reported so the dialog can name all of them at
// once), three notes, no field ever left nil (an empty slice, never null, matching this package's
// own convention elsewhere).
func ClassifyWorktreeAdd(in ClassifyWorktreeAddInput) WorktreeAddPreflight {
	blockers := []WorktreeAddBlocker{}

	if in.Path == "" || in.PathInsideGitDir {
		blockers = append(blockers, WorktreeAddBlocker{Kind: "invalidPath", Path: in.Path})
	}
	if in.PathExists {
		blockers = append(blockers, WorktreeAddBlocker{Kind: "pathExists", Path: in.Path})
	}
	if in.Mode == "existingBranch" && in.BranchCheckedOutElsewhere != nil {
		blockers = append(blockers, WorktreeAddBlocker{
			Kind: "branchCheckedOutElsewhere", Branch: in.Branch, WorktreePath: *in.BranchCheckedOutElsewhere,
		})
	}
	if in.Mode == "newBranch" && in.BranchExists {
		blockers = append(blockers, WorktreeAddBlocker{Kind: "branchExists", Branch: in.Branch})
	}
	if !in.StartPointResolves {
		startPoint := in.StartPoint
		if in.Mode == "existingBranch" {
			startPoint = in.Branch
		}
		blockers = append(blockers, WorktreeAddBlocker{Kind: "unknownStartPoint", StartPoint: startPoint})
	}

	notes := []WorktreeAddNote{}
	if in.PathInsideRepo {
		notes = append(notes, WorktreeAddNote{Kind: "pathInsideRepo", Path: in.Path})
	}
	if in.ParentDirMissing {
		notes = append(notes, WorktreeAddNote{Kind: "parentDirectoryMissing", Path: in.Path})
	}
	if in.Mode == "detach" {
		notes = append(notes, WorktreeAddNote{Kind: "detachedHead", Path: in.Path})
	}

	verdict := "clean"
	if len(blockers) > 0 {
		verdict = "blocked"
	}

	// Offered iff branchCheckedOutElsewhere is the ONLY blocker present — unlike checkout's
	// own detachHere route, which composes freely with a simultaneous dirty-tree block,
	// worktree add's own blockers (invalidPath, pathExists, branchExists, unknownStartPoint) name
	// problems detaching cannot fix at all, so offering the route alongside any of them would be
	// offering a button that cannot succeed.
	routes := []string{}
	if len(blockers) == 1 && blockers[0].Kind == "branchCheckedOutElsewhere" {
		routes = append(routes, "detachHere")
	}

	return WorktreeAddPreflight{
		Path: in.Path, Mode: in.Mode, Branch: in.Branch,
		Blockers: blockers, Notes: notes, Verdict: verdict, Routes: routes,
	}
}

// WorktreeRemoveBlocker mirrors D8's own WorktreeRemoveBlocker union exactly, flattened.
type WorktreeRemoveBlocker struct {
	Kind   string `json:"kind"` // "notAWorktree"|"mainWorktree"|"currentWorktree"|"openInAnotherWindow"|"locked"
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason,omitempty"` // locked
}

// WorktreeRemovePreflight mirrors the wire's own WorktreeRemovePreflight field for field.
type WorktreeRemovePreflight struct {
	Path                      string                  `json:"path"`
	Blockers                  []WorktreeRemoveBlocker `json:"blockers"`
	RequiresTypedConfirmation bool                    `json:"requiresTypedConfirmation"`
	// ConfirmToken: fingerprint of the uncommitted set (paths, statuses, size and mtime), set only when
	// RequiresTypedConfirmation is true. The client echoes the one it showed (the name is historic:
	// nothing is typed any more); the server re-derives it fresh and refuses a mismatch, so discard
	// deletes only the set the user saw.
	ConfirmToken string   `json:"confirmToken,omitempty"`
	Routes       []string `json:"routes"`  // "force" — offered iff dirty and otherwise unblocked.
	Verdict      string   `json:"verdict"` // "clean" | "dirty" | "blocked"
}

// ClassifyWorktreeRemoveInput is ClassifyWorktreeRemove's own input.
type ClassifyWorktreeRemoveInput struct {
	Path string
	// IsWorktree: Path matched one of `worktree list`'s own entries exactly (by absolute path).
	// false is the security check: the app must never run a destructive command against a
	// caller-supplied arbitrary path, worktree or not — this is what refuses that,
	// unconditionally, before any of the other four checks are even meaningful.
	IsWorktree          bool
	IsMainWorktree      bool
	IsCurrentWorktree   bool // this session's own repo root — F7's own cross-window hazard, this session's half.
	OpenInAnotherWindow bool // Registry.IsOpen(repoID) for the OTHER half of F7's hazard.
	LockedReason        *string
	// DirtyToken fingerprints the uncommitted set, computed host-side; "" means clean.
	DirtyToken string
}

// ClassifyWorktreeRemove is the classifier: notAWorktree first (the security gate — every
// other field is meaningless for a path that is not a real worktree at all), then
// mainWorktree/currentWorktree/openInAnotherWindow/locked, each unconditional (no force route
// unlocks any of the five — WorktreeRemoveArgs' own force flag is a single `--force`, which git
// itself only honours for the dirty case, the hint text ("use 'remove -f -f'...")
// confirms a lock needs a second, undelivered flag this app never sends). Only once ALL FIVE are
// clear does Dirty get to turn the verdict into "dirty" with a force route and a confirmation
// token — fail-safe over fail-open.
func ClassifyWorktreeRemove(in ClassifyWorktreeRemoveInput) WorktreeRemovePreflight {
	blockers := []WorktreeRemoveBlocker{}
	if !in.IsWorktree {
		blockers = append(blockers, WorktreeRemoveBlocker{Kind: "notAWorktree", Path: in.Path})
	}
	if in.IsMainWorktree {
		blockers = append(blockers, WorktreeRemoveBlocker{Kind: "mainWorktree"})
	}
	if in.IsCurrentWorktree {
		blockers = append(blockers, WorktreeRemoveBlocker{Kind: "currentWorktree"})
	}
	if in.OpenInAnotherWindow {
		blockers = append(blockers, WorktreeRemoveBlocker{Kind: "openInAnotherWindow"})
	}
	if in.LockedReason != nil {
		blockers = append(blockers, WorktreeRemoveBlocker{Kind: "locked", Reason: *in.LockedReason})
	}

	if len(blockers) > 0 {
		return WorktreeRemovePreflight{Path: in.Path, Blockers: blockers, Routes: []string{}, Verdict: "blocked"}
	}

	if in.DirtyToken != "" {
		return WorktreeRemovePreflight{
			Path: in.Path, Blockers: blockers,
			RequiresTypedConfirmation: true, ConfirmToken: in.DirtyToken,
			Routes: []string{"force"}, Verdict: "dirty",
		}
	}

	return WorktreeRemovePreflight{Path: in.Path, Blockers: blockers, Routes: []string{}, Verdict: "clean"}
}
