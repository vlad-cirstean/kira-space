package gitreview

import (
	"math"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
)

// ProjectRanges maps ranges (in the OLD/snapshot side's line numbers) forward through hunks (a
// snapshot->current patch, old = snapshot, new = current) into new-side (current/branch-tip) line
// numbers. Outside every hunk, unchanged regions shift uniformly by the running total of
// each hunk's (NewLines - OldLines); inside a hunk, only context lines have an image (a `del` line
// has none and drops out — exactly right: those lines no longer exist to be "reviewed"). The
// result is normalized and clamped to [1, newLineCount] — a stored range that outlived the file
// shrinking below it is truncated, never left dangling past EOF.
func ProjectRanges(ranges []LineRange, hunks []porcelain.DiffHunk, newLineCount int) []LineRange {
	normalized := Normalize(ranges)
	if len(normalized) == 0 {
		return nil
	}
	if len(hunks) == 0 {
		return clampToLineCount(normalized, newLineCount)
	}
	var out []LineRange
	for _, r := range normalized {
		out = append(out, projectOne(r, hunks)...)
	}
	return clampToLineCount(Normalize(out), newLineCount)
}

// projectOne projects a single old-side range through hunks (assumed sorted ascending by
// OldStart, as every real diff's hunk list is) by walking the gaps between hunks — where the
// mapping is a pure offset shift — and, for the portion inside a hunk, mapping context
// lines one at a time (bounded by that hunk's own size, never by the range's).
func projectOne(r LineRange, hunks []porcelain.DiffHunk) []LineRange {
	var out []LineRange
	offset := 0
	prevOldEnd := 0 // the last old-side line covered by any hunk seen so far; 0 = "before the file"

	for _, h := range hunks {
		hunkOldStart := h.OldStart
		hunkOldEnd := h.OldStart + h.OldLines - 1 // < hunkOldStart for a pure insertion (OldLines == 0)

		if s, e, ok := intersect(r, prevOldEnd+1, hunkOldStart-1); ok {
			out = append(out, LineRange{Start: s + offset, End: e + offset})
		}
		if s, e, ok := intersect(r, hunkOldStart, hunkOldEnd); ok {
			out = append(out, mapWithinHunk(h, s, e)...)
		}

		offset += h.NewLines - h.OldLines
		if hunkOldEnd > prevOldEnd {
			prevOldEnd = hunkOldEnd
		}
	}

	if s, e, ok := intersect(r, prevOldEnd+1, math.MaxInt); ok {
		out = append(out, LineRange{Start: s + offset, End: e + offset})
	}
	return out
}

// intersect returns r ∩ [lo, hi], reporting false when the intersection is empty.
func intersect(r LineRange, lo, hi int) (start, end int, ok bool) {
	start = max(r.Start, lo)
	end = min(r.End, hi)
	if start > end {
		return 0, 0, false
	}
	return start, end, true
}

// mapWithinHunk maps every context line inside h whose OldLine falls in [lo, hi] to its NewLine,
// then folds the resulting (already old-line-ordered, and therefore new-line-ordered) points into
// ranges — consecutive surviving lines produce one contiguous span even when a deletion sits
// between their OLD line numbers, since what matters is that nothing was inserted between their
// NEW ones.
func mapWithinHunk(h porcelain.DiffHunk, lo, hi int) []LineRange {
	var points []int
	for _, line := range h.Lines {
		if line.Kind != porcelain.LineContext || line.OldLine == nil {
			continue
		}
		if *line.OldLine < lo || *line.OldLine > hi {
			continue
		}
		points = append(points, *line.NewLine)
	}
	return pointsToRanges(points)
}

func pointsToRanges(points []int) []LineRange {
	if len(points) == 0 {
		return nil
	}
	out := make([]LineRange, 0, len(points))
	start, prev := points[0], points[0]
	for _, p := range points[1:] {
		if p == prev+1 {
			prev = p
			continue
		}
		out = append(out, LineRange{Start: start, End: prev})
		start, prev = p, p
	}
	return append(out, LineRange{Start: start, End: prev})
}

// clampToLineCount drops ranges entirely past newLineCount and truncates ones that partially
// exceed it — a stored range projected against a file that has since shrunk must never claim a
// line number that no longer exists.
func clampToLineCount(ranges []LineRange, newLineCount int) []LineRange {
	if newLineCount <= 0 {
		return nil
	}
	out := make([]LineRange, 0, len(ranges))
	for _, r := range ranges {
		if r.Start > newLineCount {
			continue
		}
		end := r.End
		if end > newLineCount {
			end = newLineCount
		}
		out = append(out, LineRange{Start: r.Start, End: end})
	}
	return Normalize(out)
}

// DeletedRanges returns the part of ranges (snapshot coordinates) that a snapshot->current patch
// deletes: every old line a `del` line removes, intersected with ranges. A line rewritten in place
// (del plus add) counts as deleted.
func DeletedRanges(ranges []LineRange, hunks []porcelain.DiffHunk) []LineRange {
	var del []LineRange
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Kind == porcelain.LineDel && l.OldLine != nil {
				del = append(del, LineRange{Start: *l.OldLine, End: *l.OldLine})
			}
		}
	}
	return Intersect(Normalize(ranges), Normalize(del))
}
