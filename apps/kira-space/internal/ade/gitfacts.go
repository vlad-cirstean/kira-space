package ade

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Git fact helpers the v2 TaskBoard shares: ref resolution, dirty mapping, range facts, per-repo caches.

// adeRepoChangedDebounce is §5.1's own "Signal" rule: a held repo's own repo.changed is coalesced
// per repo before OnRepoChanged fires — a fetch or a background ref move commonly touches several
// refs in a burst.
const adeRepoChangedDebounce = 250 * time.Millisecond

// branchFactsCacheCap/mergeTreeCacheCap are the "512 entries per repo" cache bound, applied
// to two separate per-repo LRUs (ahead/behind/files/commits keyed by (tip(parent), tip(branch));
// merge-tree conflicts keyed by the ordered pair of tips pairFacts already calls its callback with)
// rather than one cache discriminated by a third "kind" field — tips are immutable object ids, so
// neither needs an invalidation rule beyond eviction.
const (
	branchFactsCacheCap = 512
	mergeTreeCacheCap   = 512
	patchIDCacheCap     = 32
)

type DirtyEntry struct{ Code, Path string }

type PairFact struct {
	A, B      string
	Shared    []string
	Conflicts []string
}

type branchFactsKey struct{ TipParent, TipBranch string }

type branchFactsValue struct {
	Ahead, Behind int
	Files         []porcelain.FileChange
}

type mergeTreeKey struct{ TipA, TipB string }

type repoCaches struct {
	branch    *lru.Cache[branchFactsKey, branchFactsValue]
	mergeTree *lru.Cache[mergeTreeKey, []string]
	contain   *lru.Cache[containKey, containment]
	patchIDs  *lru.Cache[string, map[string]struct{}] // target tip -> patch ids of its recent commits
	commits   *lru.Cache[string, string]              // printed sha -> full commit id (found shas only)
	ancestor  *lru.Cache[tipPair, bool]               // (older, newer) -> older is an ancestor of newer
	since     *lru.Cache[tipPair, int]                // (older, newer) -> commits in older..newer
	counts    *lru.Cache[tipPair, [2]int]             // (left, right) -> ahead, behind of left...right

	// ancestry holds the last computeAncestry answer: one slot, keyed by a signature over its input.
	ancestryMu  sync.Mutex
	ancestrySig string
	ancestry    map[string][]string
}

// tipPair keys a git fact on two immutable commit ids.
type tipPair struct{ from, to string }

func newRepoCaches() *repoCaches {
	b, _ := lru.New[branchFactsKey, branchFactsValue](branchFactsCacheCap)
	m, _ := lru.New[mergeTreeKey, []string](mergeTreeCacheCap)
	c, _ := lru.New[containKey, containment](branchFactsCacheCap)
	p, _ := lru.New[string, map[string]struct{}](patchIDCacheCap)
	r, _ := lru.New[string, string](patchIDCacheCap)
	a, _ := lru.New[tipPair, bool](branchFactsCacheCap)
	n, _ := lru.New[tipPair, int](branchFactsCacheCap)
	k, _ := lru.New[tipPair, [2]int](branchFactsCacheCap)
	return &repoCaches{branch: b, mergeTree: m, contain: c, patchIDs: p, commits: r, ancestor: a, since: n, counts: k}
}

// resolveQueuedRef is §0.14's own identity rule: refs/heads/<b> when present, else
// refs/remotes/<defaultRemote>/<b> (review branches are often remote-only).
func resolveQueuedRef(inventory []porcelain.InventoryRef, short, defaultRemote string) (porcelain.InventoryRef, bool) {
	for _, r := range inventory {
		if r.Remote == "" && r.Short == short {
			return r, true
		}
	}
	if defaultRemote != "" {
		for _, r := range inventory {
			if r.Remote == defaultRemote && r.Short == short {
				return r, true
			}
		}
	}
	return porcelain.InventoryRef{}, false
}

func dirtyCode(e porcelain.StatusEntry) (string, bool) {
	if e.Kind == "ignored" {
		return "", false
	}
	if e.Kind == "untracked" {
		return "??", true
	}
	if e.Staged == 'D' || e.Unstaged == 'D' {
		return "D", true
	}
	return "M", true
}

func toDirtyEntries(entries []porcelain.StatusEntry) []DirtyEntry {
	out := make([]DirtyEntry, 0, len(entries))
	for _, e := range entries {
		if code, ok := dirtyCode(e); ok {
			out = append(out, DirtyEntry{Code: code, Path: e.Path})
		}
	}
	return out
}

func resolveKind(row porcelain.InventoryRef, userEmail, override string) string {
	if override != "" {
		return override
	}
	if userEmail != "" && strings.EqualFold(row.AuthorEmail, userEmail) {
		return model.AdeBranchKindMine
	}
	return model.AdeBranchKindReview
}

// resolvedRef is the board's per-branch ref resolution result.
type resolvedRef struct {
	row   porcelain.InventoryRef
	found bool
}

// resolveBaseRow is the identity rule for a base named by a short ref: the default remote's branch
// when it exists, else the local one (a base is what the team shares, not a stale local twin).
func resolveBaseRow(inventory []porcelain.InventoryRef, short, defaultRemote string) (porcelain.InventoryRef, bool) {
	if defaultRemote != "" {
		if row, ok := findRemoteRow(inventory, defaultRemote, short); ok {
			return row, true
		}
	}
	for _, r := range inventory {
		if r.Remote == "" && r.Short == short {
			return r, true
		}
	}
	return porcelain.InventoryRef{}, false
}

// mainDisplay is the short-form split of MainRef's full refname:
// `refs/heads/X` -> name X, ref X (a local-only main); `refs/remotes/<r>/X` -> name X, ref
// `<r>/X` (a remote-tracking main, the common case); only this display pair is shortened. A refname
// this doesn't recognize (never
// produced by MainRef today) passes through unchanged in both fields, rather than panicking.
func mainDisplay(full string) (name, ref string) {
	if rest, ok := strings.CutPrefix(full, "refs/heads/"); ok {
		return rest, rest
	}
	if rest, ok := strings.CutPrefix(full, "refs/remotes/"); ok {
		if i := strings.IndexByte(rest, '/'); i > 0 {
			return rest[i+1:], rest
		}
	}
	return full, full
}

// aheadBehind is entry.AheadBehind read through the per-repo cache. Both ids are immutable commits,
// so an entry never goes stale. caches may be nil (no caching).
func aheadBehind(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, left, right string) (int, int, error) {
	key := tipPair{from: left, to: right}
	if caches != nil {
		if v, ok := caches.counts.Get(key); ok {
			return v[0], v[1], nil
		}
	}
	ahead, behind, err := entry.AheadBehind(ctx, left, right)
	if err != nil {
		return 0, 0, err
	}
	if caches != nil {
		caches.counts.Add(key, [2]int{ahead, behind})
	}
	return ahead, behind, nil
}

// ancestrySignature keys computeAncestry on its whole input: the sorted (item, ref, tip) triples.
func ancestrySignature(existing []string, tips, refs map[string]string) string {
	items := slices.Clone(existing)
	slices.Sort(items)
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(it + "\x00" + refs[it] + "\x00" + tips[it] + "\x01")
	}
	return sb.String()
}

func cloneAncestry(m map[string][]string) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = slices.Clone(v)
	}
	return out
}

// computeAncestry is §0.5's own Ancestors input: for each existing item, which OTHER existing
// items are strict (non-equal-tip) ancestors of it — also pairFacts' own isAncestor callback data.
func computeAncestry(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, existing []string, tips, refs map[string]string) (map[string][]string, error) {
	sig := ancestrySignature(existing, tips, refs)
	caches.ancestryMu.Lock()
	if caches.ancestry != nil && caches.ancestrySig == sig {
		out := cloneAncestry(caches.ancestry)
		caches.ancestryMu.Unlock()
		return out, nil
	}
	caches.ancestryMu.Unlock()
	ancestryOf, err := queryAncestry(ctx, entry, existing, tips, refs)
	if err != nil {
		return nil, err
	}
	caches.ancestryMu.Lock()
	caches.ancestrySig, caches.ancestry = sig, cloneAncestry(ancestryOf)
	caches.ancestryMu.Unlock()
	return ancestryOf, nil
}

func queryAncestry(ctx context.Context, entry *gitsession.RepoEntry, existing []string, tips, refs map[string]string) (map[string][]string, error) {
	ancestryOf := make(map[string][]string, len(existing))
	refToItem := make(map[string]string, len(existing))
	for _, item := range existing {
		refToItem[refs[item]] = item
	}
	for _, item := range existing {
		var among []string
		for _, other := range existing {
			if other != item {
				among = append(among, refs[other])
			}
		}
		if len(among) == 0 {
			continue
		}
		merged, err := entry.Ancestors(ctx, tips[item], among)
		if err != nil {
			return nil, err
		}
		for _, refname := range merged {
			other := refToItem[refname]
			if tips[other] == tips[item] {
				continue // equal tip: never a strict ancestor (branchNode.Ancestors' own contract).
			}
			ancestryOf[item] = append(ancestryOf[item], other)
		}
	}
	return ancestryOf, nil
}

func findRemoteRow(inventory []porcelain.InventoryRef, remote, short string) (porcelain.InventoryRef, bool) {
	for _, rr := range inventory {
		if rr.Remote == remote && rr.Short == short {
			return rr, true
		}
	}
	return porcelain.InventoryRef{}, false
}

// remoteTwin is a local row's remote counterpart and the commit counts against it: the tracked
// upstream row with git's own counts, else the default remote's branch of the same short name with
// counts from AheadBehind. ok is false when there is none, or the tracked upstream is gone.
func remoteTwin(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, inv []porcelain.InventoryRef, row porcelain.InventoryRef, defaultRemote string) (twin porcelain.InventoryRef, ahead, behind int, ok bool, err error) {
	if row.Upstream != "" {
		if row.UpstreamGone {
			return porcelain.InventoryRef{}, 0, 0, false, nil
		}
		for _, r := range inv {
			if r.Ref == row.Upstream {
				return r, row.UpstreamAhead, row.UpstreamBehind, true, nil
			}
		}
		return porcelain.InventoryRef{}, 0, 0, false, nil
	}
	if defaultRemote == "" {
		return porcelain.InventoryRef{}, 0, 0, false, nil
	}
	twin, found := findRemoteRow(inv, defaultRemote, row.Short)
	if !found {
		return porcelain.InventoryRef{}, 0, 0, false, nil
	}
	ahead, behind, err = aheadBehind(ctx, entry, caches, row.Tip, twin.Tip)
	if err != nil {
		return porcelain.InventoryRef{}, 0, 0, false, err
	}
	return twin, ahead, behind, true, nil
}

// rangeFacts is the (parentTip, tip)-keyed LRU cache (§4.2's own 512-entry bound) around
// ahead/behind/files — the one git-cost-bearing step the TaskBoard
// repeats often enough (every board build, every branch) to be worth caching on immutable object ids.
func rangeFacts(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, parentTip, tip string) (branchFactsValue, error) {
	key := branchFactsKey{TipParent: parentTip, TipBranch: tip}
	if v, ok := caches.branch.Get(key); ok {
		return v, nil
	}
	ahead, behind, err := entry.AheadBehind(ctx, tip, parentTip)
	if err != nil {
		return branchFactsValue{}, err
	}
	files, err := entry.RangeChanges(ctx, parentTip, tip)
	if err != nil {
		return branchFactsValue{}, err
	}
	v := branchFactsValue{Ahead: ahead, Behind: behind, Files: files}
	caches.branch.Add(key, v)
	return v, nil
}

// computePairFacts wraps pairFacts (§0.7) with its own isAncestor/mergeTree callbacks — the
// mergeTree side is the (tipA, tipB)-keyed LRU cache (§4.2), so a repeat pair whose tips are
// unchanged since the last board build or refresh never re-spawns merge-tree.
func computePairFacts(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, ancestryOf map[string][]string, pairItems []pairItem) ([]PairFact, error) {
	isAncestorFn := func(x, y string) bool {
		for _, a := range ancestryOf[y] {
			if a == x {
				return true
			}
		}
		for _, a := range ancestryOf[x] {
			if a == y {
				return true
			}
		}
		return false
	}
	mergeTreeFn := func(tipA, tipB string) ([]string, error) {
		key := mergeTreeKey{TipA: tipA, TipB: tipB}
		if v, ok := caches.mergeTree.Get(key); ok {
			return v, nil
		}
		pred, err := entry.MergeTreeConflicts(ctx, tipA, tipB)
		if err != nil {
			return nil, err
		}
		paths := pred.Paths
		if paths == nil {
			paths = []string{}
		}
		caches.mergeTree.Add(key, paths)
		return paths, nil
	}
	pairs, err := pairFacts(pairItems, isAncestorFn, mergeTreeFn)
	if err != nil {
		return nil, err
	}
	out := make([]PairFact, len(pairs))
	for i, p := range pairs {
		out[i] = PairFact{A: p.A, B: p.B, Shared: p.Shared, Conflicts: p.Conflicts}
	}
	return out, nil
}

func filesSet(changes []porcelain.FileChange) map[string]bool {
	out := make(map[string]bool, len(changes))
	for _, c := range changes {
		out[c.Path] = true
	}
	return out
}

func forcePushRemote(ctx context.Context, entry *gitsession.RepoEntry, branch string) string {
	if inv, err := entry.BranchInventory(ctx); err == nil {
		for _, r := range inv {
			if r.Remote == "" && r.Short == branch && r.Upstream != "" {
				rest := strings.TrimPrefix(r.Upstream, "refs/remotes/")
				if i := strings.IndexByte(rest, '/'); i > 0 {
					return rest[:i]
				}
			}
		}
	}
	remote, _ := entry.DefaultRemote(ctx)
	return remote
}
