package codeworkspace

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

// scanContent writes content to a fresh temp file and runs scanFile against it — every test below
// drives the real gates (ValidateRelPath, stat, binary sniff, bufio.Scanner) rather than calling
// buildPreview/utf16Units in isolation, except where a test is specifically about buildPreview's
// own boundary arithmetic (TestBuildPreview... below).
func scanContent(t *testing.T, content []byte, req SearchRequest) (matches []SearchMatch, truncated, skipped bool) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	m, err := newMatcher(req)
	if err != nil {
		t.Fatalf("newMatcher(%+v): %v", req, err)
	}
	buf := make([]byte, 0, 64*1024)
	svc, err := Files(dir)
	if err != nil {
		t.Fatal(err)
	}
	return scanFile(svc, "f.txt", m, buf)
}

func TestScanFile_LiteralMatchesOnOneLine(t *testing.T) {
	matches, truncated, skipped := scanContent(t, []byte("foo bar foo\n"),
		SearchRequest{Query: "foo", CaseSensitive: true})
	if skipped || truncated {
		t.Fatalf("skipped=%v truncated=%v, want both false", skipped, truncated)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(matches), matches)
	}
	want := []SearchMatch{
		{Line: 1, Column: 1, EndColumn: 4, Preview: "foo bar foo", PreviewMatchStart: 0, PreviewMatchEnd: 3},
		{Line: 1, Column: 9, EndColumn: 12, Preview: "foo bar foo", PreviewMatchStart: 8, PreviewMatchEnd: 11},
	}
	for i, w := range want {
		got := matches[i]
		if got.Line != w.Line || got.Column != w.Column || got.EndColumn != w.EndColumn ||
			got.Preview != w.Preview || got.PreviewMatchStart != w.PreviewMatchStart ||
			got.PreviewMatchEnd != w.PreviewMatchEnd {
			t.Fatalf("match %d = %+v, want %+v", i, got, w)
		}
	}
}

// TestScanFile_ColumnAgreesWithLineIndex is the cross-check: a match's Column must equal
// exactly what LineIndex.Position gives for the same byte, for a match landing right after a
// 3-byte rune (€, one UTF-16 unit) and right after a surrogate-pair emoji (😀, two UTF-16 units) —
// the two genuinely different cases rule 2 distinguishes.
func TestScanFile_ColumnAgreesWithLineIndex(t *testing.T) {
	content := "€x\n😀x\n"
	matches, truncated, skipped := scanContent(t, []byte(content), SearchRequest{Query: "x", CaseSensitive: true})
	if skipped || truncated {
		t.Fatalf("skipped=%v truncated=%v, want both false", skipped, truncated)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(matches), matches)
	}

	li := NewLineIndex([]byte(content))
	cases := []struct {
		row, byteCol int
		got          SearchMatch
	}{
		{0, len("€"), matches[0]}, // 'x' right after the 3-byte rune
		{1, len("😀"), matches[1]}, // 'x' right after the surrogate-pair rune
	}
	for _, c := range cases {
		wantLine, wantCol := li.Position(c.row, c.byteCol)
		if c.got.Line != wantLine || c.got.Column != wantCol {
			t.Fatalf("row %d: scanFile gave Line=%d Column=%d, LineIndex.Position gave %d/%d",
				c.row, c.got.Line, c.got.Column, wantLine, wantCol)
		}
	}
}

func TestScanFile_CRLFStripsCarriageReturn(t *testing.T) {
	matches, _, skipped := scanContent(t, []byte("foo\r\nbar\r\n"), SearchRequest{Query: "foo", CaseSensitive: true})
	if skipped {
		t.Fatalf("skipped, want a real scan")
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(matches), matches)
	}
	if matches[0].Preview != "foo" {
		t.Fatalf("Preview = %q, want %q (no trailing \\r)", matches[0].Preview, "foo")
	}
	if matches[0].EndColumn != 4 {
		t.Fatalf("EndColumn = %d, want 4 (the \\r must not extend it)", matches[0].EndColumn)
	}
}

// TestBuildPreview_CutsOnRuneBoundaries places multi-byte runes so a naive fixed-offset window
// (start-previewLeadBytes .. +previewMaxBytes) lands mid-rune on *both* sides — buildPreview must
// snap each side outward to a real boundary, or the returned preview is invalid UTF-8.
func TestBuildPreview_CutsOnRuneBoundaries(t *testing.T) {
	prefix := strings.Repeat("a", 301) + "あ" + strings.Repeat("b", 62)
	match := "TARGET"
	suffix := strings.Repeat("c", 440) + "い" + strings.Repeat("d", 100)
	line := prefix + match + suffix
	if len(line) <= previewMaxBytes {
		t.Fatalf("fixture line too short: %d bytes, want > %d", len(line), previewMaxBytes)
	}

	start := len(prefix)
	end := start + len(match)

	// Sanity: confirm this fixture actually exercises the snap in both directions — otherwise the
	// test below would pass even with the boundary-snapping logic deleted.
	naiveStart := start - previewLeadBytes
	if utf8.RuneStart(line[naiveStart]) {
		t.Fatalf("fixture doesn't exercise the start snap: byte %d is already a rune start", naiveStart)
	}
	naiveEnd := naiveStart + previewMaxBytes
	if naiveEnd >= len(line) || utf8.RuneStart(line[naiveEnd]) {
		t.Fatalf("fixture doesn't exercise the end snap: byte %d is already a rune start (or out of range)", naiveEnd)
	}

	preview, pmStart, pmEnd, truncStart, truncEnd := buildPreview([]byte(line), start, end)
	if !truncStart || !truncEnd {
		t.Fatalf("TruncatedStart=%v TruncatedEnd=%v, want both true", truncStart, truncEnd)
	}
	if !utf8.ValidString(preview) {
		t.Fatalf("preview is not valid UTF-8 — a cut landed mid-rune: %q", preview)
	}
	previewUnits := utf16Units([]byte(preview))
	if pmStart < 0 || pmEnd > previewUnits || pmStart > pmEnd {
		t.Fatalf("match offsets [%d,%d) out of preview's own %d UTF-16 units", pmStart, pmEnd, previewUnits)
	}
}

// TestScanFile_ZeroWidthPatternTerminates is the one genuine trap in the scan loop: a
// zero-width match (here, "x*" against a line with no 'x' at all) must advance by one byte rather
// than looping forever at the same position. This test's own failure mode, if the rule regressed,
// is a hang, not a wrong answer.
func TestScanFile_ZeroWidthPatternTerminates(t *testing.T) {
	matches, truncated, skipped := scanContent(t, []byte("abc\n"),
		SearchRequest{Query: "x*", Regex: true, CaseSensitive: true})
	if skipped || truncated {
		t.Fatalf("skipped=%v truncated=%v, want both false", skipped, truncated)
	}
	// One zero-width match at each of the 4 positions in "abc" (before a, before b, before c, and
	// at the end of line).
	if len(matches) != 4 {
		t.Fatalf("got %d matches, want 4: %+v", len(matches), matches)
	}
	for i, m := range matches {
		if m.Column != m.EndColumn {
			t.Fatalf("match %d: Column=%d EndColumn=%d, want equal (zero-width)", i, m.Column, m.EndColumn)
		}
	}
}

func TestScanFile_PerFileCapTruncates(t *testing.T) {
	var b strings.Builder
	for range MaxMatchesPerFile + 50 {
		b.WriteString("x\n")
	}
	matches, truncated, skipped := scanContent(t, []byte(b.String()), SearchRequest{Query: "x", CaseSensitive: true})
	if skipped {
		t.Fatalf("skipped, want a real scan")
	}
	if !truncated {
		t.Fatalf("truncated=false, want true (the file has more than MaxMatchesPerFile matches)")
	}
	if len(matches) != MaxMatchesPerFile {
		t.Fatalf("got %d matches, want exactly MaxMatchesPerFile=%d", len(matches), MaxMatchesPerFile)
	}
}

func TestScanFile_NULInFirst8KiBIsBinary(t *testing.T) {
	content := append([]byte("TARGET line\n"), 0x00)
	content = append(content, []byte(strings.Repeat("filler\n", 100))...)
	_, _, skipped := scanContent(t, content, SearchRequest{Query: "TARGET", CaseSensitive: true})
	if !skipped {
		t.Fatalf("skipped=false, want true (a NUL in the first 8 KiB marks the file binary)")
	}
}

// TestScanFile_NULOnlyOnMatchLineDropsWholeFile: a file whose first 8 KiB is plain ASCII
// (so the sniff passes it as text) but whose body later contains a NUL on a line that would
// otherwise be a match must be dropped whole — including a match already collected earlier in the
// same file.
func TestScanFile_NULOnlyOnMatchLineDropsWholeFile(t *testing.T) {
	var b strings.Builder
	b.WriteString("TARGET early match\n")
	// Well past the first 8 KiB, all plain ASCII, no match — keeps gate 3's sniff clean.
	b.WriteString(strings.Repeat("filler line, no match here\n", 400))
	b.WriteString("TARGET")
	b.WriteByte(0x00)
	b.WriteString("rest\n")

	_, _, skipped := scanContent(t, []byte(b.String()), SearchRequest{Query: "TARGET", CaseSensitive: true})
	if !skipped {
		t.Fatalf("skipped=false, want true (a NUL on a match line drops the whole file)")
	}
}

func TestScanFile_LineOverMaxLengthSkipsWholeFile(t *testing.T) {
	content := []byte("TARGET\n" + strings.Repeat("x", maxSearchLineBytes+1) + "\n")
	_, _, skipped := scanContent(t, content, SearchRequest{Query: "TARGET", CaseSensitive: true})
	if !skipped {
		t.Fatalf("skipped=false, want true (a line over maxSearchLineBytes skips the whole file)")
	}
}

// TestScanFile_RegexAnchorsPerLine is D5: ^ and $ anchor to each line, never to the whole file —
// a match spanning a line boundary is out of scope by construction, since the scanner hands the
// matcher one line at a time.
func TestScanFile_RegexAnchorsPerLine(t *testing.T) {
	matches, _, skipped := scanContent(t, []byte("abc\nxabcx\nabc\n"),
		SearchRequest{Query: "^abc$", Regex: true, CaseSensitive: true})
	if skipped {
		t.Fatalf("skipped, want a real scan")
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2 (lines 1 and 3 only, not the substring on line 2): %+v", len(matches), matches)
	}
	if matches[0].Line != 1 || matches[1].Line != 3 {
		t.Fatalf("matched lines = [%d,%d], want [1,3]", matches[0].Line, matches[1].Line)
	}
}

// TestScanFile_RegexAnchorsWithinLine: ^ must anchor to the real start of the line even
// past an earlier match on that same line, not to wherever the previous match happened to end. The
// old find-and-reslice loop re-sliced the line at each match's end and re-ran the regex against
// that slice, which made "^" look true again at the slice's own start — wrongly reporting two
// matches of "^import" against "importimport" instead of one.
func TestScanFile_RegexAnchorsWithinLine(t *testing.T) {
	matches, _, skipped := scanContent(t, []byte("importimport\n"),
		SearchRequest{Query: "^import", Regex: true, CaseSensitive: true})
	if skipped {
		t.Fatalf("skipped, want a real scan")
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want exactly 1: %+v", len(matches), matches)
	}
	if matches[0].Column != 1 || matches[0].EndColumn != 7 {
		t.Fatalf("match = [%d,%d], want [1,7] (only the line's real start)", matches[0].Column, matches[0].EndColumn)
	}
}

// TestScanFile_WordBoundaryWithinLine: the same re-slicing bug made a `\b`
// alternative in the pattern falsely match mid-word, since the reslice presented a false "start of
// line" as a false word boundary too. "bar|\bfoo" against "barfoo foo" must match only the real
// "bar" at the start and the real, boundary-preceded "foo" at the end — never the "foo" embedded
// inside "barfoo".
func TestScanFile_WordBoundaryWithinLine(t *testing.T) {
	matches, _, skipped := scanContent(t, []byte("barfoo foo\n"),
		SearchRequest{Query: `bar|\bfoo`, Regex: true, CaseSensitive: true})
	if skipped {
		t.Fatalf("skipped, want a real scan")
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want exactly 2: %+v", len(matches), matches)
	}
	if matches[0].Column != 1 || matches[0].EndColumn != 4 {
		t.Fatalf("match[0] = [%d,%d], want [1,4] (\"bar\" at the start)", matches[0].Column, matches[0].EndColumn)
	}
	if matches[1].Column != 8 || matches[1].EndColumn != 11 {
		t.Fatalf("match[1] = [%d,%d], want [8,11] (\"foo\" at the end, not the mid-word one)", matches[1].Column, matches[1].EndColumn)
	}
}

// TestScanFile_FIFO_SkippedWithoutHanging is the search-side twin of
// TestReadFile_FIFO_ReturnsMissingWithoutHanging (files_test.go) — gate 2's os.Stat now rejects a
// non-regular file before scanFile ever calls os.Open, which is the only ordering that avoids
// the hang (os.Open, not os.Stat, is what blocks opening a FIFO with no writer).
func TestScanFile_FIFO_SkippedWithoutHanging(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}

	m, err := newMatcher(SearchRequest{Query: "foo", CaseSensitive: true})
	if err != nil {
		t.Fatalf("newMatcher: %v", err)
	}

	svc, err := Files(dir)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		matches   []SearchMatch
		truncated bool
		skipped   bool
	}
	done := make(chan result, 1)
	go func() {
		matches, truncated, skipped := scanFile(svc, "pipe", m, make([]byte, 0, 64*1024))
		done <- result{matches, truncated, skipped}
	}()

	select {
	case r := <-done:
		if !r.skipped {
			t.Fatalf("scanFile(FIFO): skipped = false, want true")
		}
		if len(r.matches) != 0 || r.truncated {
			t.Fatalf("scanFile(FIFO) = %+v, want zero matches, not truncated", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scanFile(FIFO) did not return within 2s — it hung opening the FIFO")
	}
}
