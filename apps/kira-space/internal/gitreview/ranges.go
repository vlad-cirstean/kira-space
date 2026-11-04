package gitreview

import "sort"

// LineRange is a 1-based, inclusive line range — the wire's own shape (gitrpc's LineRange mirrors
// this field for field) and the coordinate system every review_range row and every projected
// result uses.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Normalize sorts ranges by Start and merges every overlapping or touching pair into the minimal
// equivalent set. A range with End < Start is dropped — never produced by this package's own
// callers, so a silent drop (rather than a panic) is the right answer for one that somehow arrives
// anyway.
func Normalize(ranges []LineRange) []LineRange {
	clean := make([]LineRange, 0, len(ranges))
	for _, r := range ranges {
		if r.End >= r.Start {
			clean = append(clean, r)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i].Start < clean[j].Start })
	out := []LineRange{clean[0]}
	for _, r := range clean[1:] {
		last := &out[len(out)-1]
		if r.Start <= last.End+1 { // touching (adjacent) or overlapping — one merged span
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// Union returns the normalized union of a and b.
func Union(a, b []LineRange) []LineRange {
	combined := make([]LineRange, 0, len(a)+len(b))
	combined = append(combined, a...)
	combined = append(combined, b...)
	return Normalize(combined)
}

// Subtract returns a's lines with every line in b removed, normalized. b need not be normalized
// itself — it is normalized internally before the sweep.
func Subtract(a, b []LineRange) []LineRange {
	bn := Normalize(b)
	var out []LineRange
	for _, ar := range Normalize(a) {
		cur := ar
		for _, br := range bn {
			if cur.Start > cur.End {
				break // fully consumed by an earlier br
			}
			if br.End < cur.Start || br.Start > cur.End {
				continue // no overlap with what remains of cur
			}
			if br.Start > cur.Start {
				out = append(out, LineRange{Start: cur.Start, End: br.Start - 1})
			}
			if br.End < cur.End {
				cur.Start = br.End + 1
			} else {
				cur.Start = cur.End + 1 // consumed through (at least) cur.End
			}
		}
		if cur.Start <= cur.End {
			out = append(out, cur)
		}
	}
	return Normalize(out)
}

// Intersect returns a ∩ b, normalized.
func Intersect(a, b []LineRange) []LineRange {
	return Subtract(a, Subtract(a, b))
}

// Expand turns the "full" state into its concrete range — "Expand is the only thing that turns
// [state=full] into a materialised [1..N]", used solely by an unmark of part of a fully-reviewed
// file (the one path that needs a concrete range to subtract from). lineCount <= 0 (a non-text
// record, or a genuinely empty file) expands to nothing.
func Expand(lineCount int) []LineRange {
	if lineCount <= 0 {
		return nil
	}
	return []LineRange{{Start: 1, End: lineCount}}
}

// CountLines sums the (normalized) line count covered by ranges — used only where a caller needs
// the total rather than the shape (never on the wire, which always carries ranges themselves).
func CountLines(ranges []LineRange) int {
	total := 0
	for _, r := range Normalize(ranges) {
		total += r.End - r.Start + 1
	}
	return total
}
