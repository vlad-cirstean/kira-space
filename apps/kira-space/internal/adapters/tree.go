package adapters

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// This file hoists the two path-depth error shapes postgres/mysqlfamily/sqlite's own Children each
// spelled out verbatim. The depth-by-depth dispatch itself — segment-Kind checks
// interleaved with per-level listing calls — stays in each adapter: postgres acquires its
// per-database *pgx.Conn once and reuses it through both its schema and relation listing (a depth
// mysqlfamily/sqlite have no equivalent of), while sqlite wraps the whole call in runOnConn rather
// than an explicit acquire/release pair. Folding that dispatch into one shared walker would have to
// either invent a connection-acquisition step none of the three share, or silently add or
// drop the "acquire a connection even for a leaf path" side effect each adapter's own dispatch
// order currently gives it — a real behavioral risk for no length win, so it was declined.

// UnexpectedPathKind is Children's own "this segment's Kind doesn't match what this depth expects"
// error, byte-identical text across all three relational adapters: depth 0 (the very first segment)
// says "root path segment kind"; every deeper depth says "path segment kind at depth N".
func UnexpectedPathKind(depth int, kind string) error {
	if depth == 0 {
		return New(CodeNotFound, "unexpected root path segment kind: "+kind, nil)
	}
	return Contract(fmt.Sprintf("unexpected path segment kind at depth %d: %s", depth, kind), nil)
}

// LeafChildren is Children's own leaf-depth check, byte-identical across all three relational
// adapters (Rule 5, P19 D5: every relation is a leaf) — a segment whose Kind is in leafKinds
// returns no children; anything else at that depth is CodeNotFound "unexpected object kind".
func LeafChildren(kind string, leafKinds map[string]bool) (TreeChildren, error) {
	if leafKinds[kind] {
		return TreeChildren{Nodes: []model.TreeNode{}}, nil
	}
	return TreeChildren{}, New(CodeNotFound, "unexpected object kind: "+kind, nil)
}

// PathPart names one path.Segments position RequirePath checks: Label is the descriptive
// word that position contributes to the error message; Kinds is the set of Kind values accepted
// there — nil accepts any Kind, for a segment whose caller re-checks or dispatches on its own Kind
// afterward (e.g. postgres's own Describe/Definition, which branch on the object segment's Kind
// themselves).
type PathPart struct {
	Label string
	Kinds []string
}

// Seg is the common PathPart: Kinds defaults to {label} — one position accepting exactly one Kind,
// spelled the same as its own message label. Pass extra kinds to accept a set while keeping label's
// descriptive word (e.g. Seg("table", "table", "view", "matview")).
func Seg(label string, kinds ...string) PathPart {
	if len(kinds) == 0 {
		kinds = []string{label}
	}
	return PathPart{Label: label, Kinds: kinds}
}

// AnySeg is a PathPart that accepts any Kind at that position.
func AnySeg(label string) PathPart {
	return PathPart{Label: label}
}

// RequirePath checks path has exactly len(parts) segments, each one's Kind within that position's
// accepted set, and returns the segments. Unifies the ~15 adapter-side
// requireXSegmentPath/requireRelationPath/resolveXTarget helpers that each spelled out a
// fixed-depth path check by hand, drifting on message format ("got depth N" vs "got: <path>") along
// the way — this always reports "<op> requires a <label>/<label>/... path, got: <path>".
func RequirePath(path model.NodePath, op string, parts ...PathPart) ([]model.PathSegment, error) {
	if len(path.Segments) == len(parts) && kindsMatch(path.Segments, parts) {
		return path.Segments, nil
	}
	return nil, pathErr(path, op, parts)
}

// RequirePathPrefix is RequirePath's rooted variant: path may be deeper than len(parts), only its
// first len(parts) segments are checked. For ops that address a container by its root (s3 and redis
// mutate: the renderer sends an object, prefix or key path below the bucket/database).
func RequirePathPrefix(path model.NodePath, op string, parts ...PathPart) ([]model.PathSegment, error) {
	if len(path.Segments) >= len(parts) && kindsMatch(path.Segments, parts) {
		return path.Segments, nil
	}
	return nil, pathErr(path, op, parts)
}

func kindsMatch(segs []model.PathSegment, parts []PathPart) bool {
	for i, part := range parts {
		if len(part.Kinds) == 0 {
			continue
		}
		if !slices.Contains(part.Kinds, segs[i].Kind) {
			return false
		}
	}
	return true
}

func pathErr(path model.NodePath, op string, parts []PathPart) error {
	labels := make([]string, len(parts))
	for i, part := range parts {
		labels[i] = part.Label
	}
	return New(CodeNotFound, op+" requires a "+strings.Join(labels, "/")+" path, got: "+model.EncodePath(path.Segments), nil)
}
