package gitreview

import (
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
)

func ctxLine(old, new int) porcelain.DiffLine {
	o, n := old, new
	return porcelain.DiffLine{Kind: porcelain.LineContext, OldLine: &o, NewLine: &n}
}

func delLine(old int) porcelain.DiffLine {
	o := old
	return porcelain.DiffLine{Kind: porcelain.LineDel, OldLine: &o}
}

func addLine(new int) porcelain.DiffLine {
	n := new
	return porcelain.DiffLine{Kind: porcelain.LineAdd, NewLine: &n}
}

// TestProjectRanges is D18's own decision-structure test: F7's old->new arithmetic over every
// scenario the plan names by name.
func TestProjectRanges(t *testing.T) {
	// One hunk replacing old lines 10-10 with new lines 10-12 (a net +2 insertion at line 10),
	// surrounded by context on both sides: old 8,9 -> new 8,9 (unchanged before), old 11,12 ->
	// new 13,14 (shifted by +2 after).
	growHunk := porcelain.DiffHunk{
		OldStart: 8, OldLines: 5, NewStart: 8, NewLines: 7,
		Lines: []porcelain.DiffLine{
			ctxLine(8, 8),
			ctxLine(9, 9),
			delLine(10),
			addLine(10), addLine(11), addLine(12),
			ctxLine(11, 13),
			ctxLine(12, 14),
		},
	}

	cases := []struct {
		name         string
		ranges       []LineRange
		hunks        []porcelain.DiffHunk
		newLineCount int
		want         []LineRange
	}{
		{
			name:         "empty hunk list is identity",
			ranges:       []LineRange{{5, 20}},
			hunks:        nil,
			newLineCount: 100,
			want:         []LineRange{{5, 20}},
		},
		{
			name:         "range entirely before every hunk is identity",
			ranges:       []LineRange{{1, 4}},
			hunks:        []porcelain.DiffHunk{growHunk},
			newLineCount: 100,
			want:         []LineRange{{1, 4}},
		},
		{
			name:         "range entirely after every hunk shifts by the total delta",
			ranges:       []LineRange{{20, 25}},
			hunks:        []porcelain.DiffHunk{growHunk},
			newLineCount: 100,
			want:         []LineRange{{22, 27}}, // +2 (7 new - 5 old) from the hunk
		},
		{
			name:         "range spanning a hunk projects context lines and shifts the tail",
			ranges:       []LineRange{{9, 12}},
			hunks:        []porcelain.DiffHunk{growHunk},
			newLineCount: 100,
			// old 9 -> new 9 (context); old 10 deleted, drops out; old 11,12 -> new 13,14.
			want: []LineRange{{9, 9}, {13, 14}},
		},
		{
			name:         "range containing only deleted lines drops out entirely",
			ranges:       []LineRange{{10, 10}},
			hunks:        []porcelain.DiffHunk{growHunk},
			newLineCount: 100,
			want:         nil,
		},
		{
			name: "range touching a pure-insertion hunk sees no old-side lines from it",
			// A pure insertion: OldLines == 0, so the hunk's old interval is empty (hunkOldEnd <
			// hunkOldStart) — a query range can never land "inside" it, only in the gaps around it.
			ranges: []LineRange{{4, 6}},
			hunks: []porcelain.DiffHunk{
				{OldStart: 5, OldLines: 0, NewStart: 5, NewLines: 3, Lines: []porcelain.DiffLine{addLine(5), addLine(6), addLine(7)}},
			},
			newLineCount: 100,
			// old 4 is before the insertion point (offset 0); old 5,6 are after it (offset +3).
			want: []LineRange{{4, 4}, {8, 9}},
		},
		{
			name:   "a hunk at line 1",
			ranges: []LineRange{{1, 3}},
			hunks: []porcelain.DiffHunk{
				{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 1, Lines: []porcelain.DiffLine{
					delLine(1), ctxLine(2, 1),
				}},
			},
			newLineCount: 100,
			// old 1 deleted (drops); old 2 -> new 1 (context); old 3 (after the hunk, offset -1) ->
			// new 2 — contiguous with new 1, so they merge into one span.
			want: []LineRange{{1, 2}},
		},
		{
			name:   "a hunk at EOF has no tail beyond it",
			ranges: []LineRange{{18, 20}},
			hunks: []porcelain.DiffHunk{
				{OldStart: 18, OldLines: 2, NewStart: 18, NewLines: 1, Lines: []porcelain.DiffLine{
					ctxLine(18, 18), delLine(19),
				}},
			},
			newLineCount: 18,
			// old 18 -> new 18; old 19 deleted (drops); old 20 is past EOF and clamped away entirely.
			want: []LineRange{{18, 18}},
		},
		{
			name:   "two hunks with opposite-sign deltas",
			ranges: []LineRange{{1, 30}},
			hunks: []porcelain.DiffHunk{
				// +3 net at old line 5 (a pure insertion).
				{OldStart: 5, OldLines: 0, NewStart: 6, NewLines: 3, Lines: []porcelain.DiffLine{addLine(6), addLine(7), addLine(8)}},
				// -2 net at old lines 10-11 (a pure deletion).
				{OldStart: 10, OldLines: 2, NewStart: 13, NewLines: 0, Lines: []porcelain.DiffLine{delLine(10), delLine(11)}},
			},
			newLineCount: 100,
			// old 1-4 (offset 0) -> 1-4; the insertion contributes no old-side lines; old 5-9
			// (offset +3) -> 8-12; the deletion contributes no old-side lines; old 12-30
			// (offset +3-2=+1) -> 13-31 — contiguous with the previous span (deleting lines closes
			// the gap in the NEW file's numbering), so the two merge into one.
			want: []LineRange{{1, 4}, {8, 31}},
		},
		{
			name:         "a range past newLineCount is clamped",
			ranges:       []LineRange{{90, 110}},
			hunks:        nil,
			newLineCount: 100,
			want:         []LineRange{{90, 100}},
		},
		{
			name:         "a range entirely past newLineCount is dropped",
			ranges:       []LineRange{{150, 200}},
			hunks:        nil,
			newLineCount: 100,
			want:         nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProjectRanges(tc.ranges, tc.hunks, tc.newLineCount)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ProjectRanges(%v, hunks, %d) = %v, want %v", tc.ranges, tc.newLineCount, got, tc.want)
			}
		})
	}
}

func TestProjectRangesEmptyInput(t *testing.T) {
	if got := ProjectRanges(nil, nil, 100); got != nil {
		t.Errorf("ProjectRanges(nil, nil, 100) = %v, want nil", got)
	}
}

func TestDeletedRanges(t *testing.T) {
	hunk := func(lines ...porcelain.DiffLine) []porcelain.DiffHunk {
		return []porcelain.DiffHunk{{Lines: lines}}
	}
	cases := []struct {
		name   string
		ranges []LineRange
		hunks  []porcelain.DiffHunk
		want   []LineRange
	}{
		{"no hunks", []LineRange{{1, 5}}, nil, nil},
		{"pure insertion", []LineRange{{1, 5}}, hunk(addLine(3)), nil},
		{"deletion inside", []LineRange{{2, 6}}, hunk(delLine(3), delLine(4)), []LineRange{{3, 4}}},
		{"deletion straddling", []LineRange{{4, 6}}, hunk(delLine(3), delLine(4), delLine(5)), []LineRange{{4, 5}}},
		{"deletion outside", []LineRange{{4, 6}}, hunk(delLine(1), delLine(9)), nil},
		{"replace counts as deleted", []LineRange{{2, 3}}, hunk(delLine(2), addLine(2)), []LineRange{{2, 2}}},
		{"whole file", []LineRange{{1, 3}}, hunk(delLine(1), delLine(2), delLine(3)), []LineRange{{1, 3}}},
		{
			"two hunks", []LineRange{{1, 10}},
			[]porcelain.DiffHunk{{Lines: []porcelain.DiffLine{delLine(2)}}, {Lines: []porcelain.DiffLine{delLine(7), delLine(8)}}},
			[]LineRange{{2, 2}, {7, 8}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DeletedRanges(c.ranges, c.hunks); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
