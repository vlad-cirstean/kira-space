package codeworkspace

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"sync"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// Go-native repository-wide text search — stdlib regexp (RE2) for matching, git ls-files
// (EnumerateAll, enumerate.go) for enumeration, and a bounded worker pool (searchWorkers below)
// for concurrency. No search library: each of those three is already answered elsewhere in this
// repo, leaving only the scanner (this file) to hand-roll.

// SearchRequest is the wire shape a repository-wide search runs against — Monaco's own find-widget
// vocabulary (case/whole-word/regex), so the panel's options read the same as the in-file surface.
type SearchRequest struct {
	Query         string `json:"query"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
}

// SearchMatch is one match within one file — Line/Column/EndColumn are Monaco's own 1-based line
// and 1-based UTF-16 column, Preview is the line's own text (EOL stripped, possibly windowed)
// with PreviewMatchStart/End as UTF-16 offsets *into Preview* so the renderer highlights
// without re-deriving anything.
type SearchMatch struct {
	Line              int    `json:"line"`
	Column            int    `json:"column"`
	EndColumn         int    `json:"endColumn"`
	Preview           string `json:"preview"`
	PreviewMatchStart int    `json:"previewMatchStart"`
	PreviewMatchEnd   int    `json:"previewMatchEnd"`
	TruncatedStart    bool   `json:"truncatedStart"`
	TruncatedEnd      bool   `json:"truncatedEnd"`
}

// FileMatches is one file's own group of matches, streamed to onFile as Search finds them.
type FileMatches struct {
	Path      string        `json:"path"`
	Matches   []SearchMatch `json:"matches"`
	Truncated bool          `json:"truncated"` // this file hit MaxMatchesPerFile
}

// SearchStats reports one Search run's own work — recorded in the status line, per §13's own
// posture (a number reported, not a threshold asserted).
type SearchStats struct {
	FilesScanned int  `json:"filesScanned"`
	FilesMatched int  `json:"filesMatched"`
	FilesSkipped int  `json:"filesSkipped"` // binary, tooLarge, longLine, unreadable, unsafe path
	Matches      int  `json:"matches"`
	Truncated    bool `json:"truncated"` // the run hit MaxSearchMatches and stopped early
}

const (
	// MaxSearchFileBytes reuses §8.1's viewer cap verbatim (D4): a match in a file the viewer can't
	// open is a result the user can't click, so the search never surfaces one either.
	MaxSearchFileBytes = MaxReadBytes
	// maxSearchLineBytes: a longer line means generated/minified content — the whole file is
	// skipped rather than reporting unreadable matches out of it.
	maxSearchLineBytes = 1 * 1024 * 1024
	// MaxSearchMatches is the whole-run cap (D7's coalescer flush count is unrelated and smaller).
	MaxSearchMatches = 10_000
	// MaxMatchesPerFile bounds one file's own contribution, independent of the whole-run cap.
	MaxMatchesPerFile = 500
	// previewMaxBytes/previewLeadBytes: §3.5's preview window — the lead kept before a windowed
	// match's own start.
	previewMaxBytes  = 512
	previewLeadBytes = 64
)

// matcher is the one seam between "how to find a match on a line" and everything else about the
// scan — a bytes.Index fast path for the common case, RE2 for everything else (§3.3).
type matcher interface {
	// find returns the next match in line starting no earlier than byte offset from, or
	// ok == false when there is none.
	find(line []byte, from int) (start, end int, ok bool)
}

// literalMatcher is the case-sensitive, non-regex, non-whole-word fast path — bytes.Index, no
// allocation, no RE2 overhead for the common "search for this exact text" case.
type literalMatcher struct{ pat []byte }

func (m literalMatcher) find(line []byte, from int) (start, end int, ok bool) {
	if from > len(line) {
		return 0, 0, false
	}
	idx := bytes.Index(line[from:], m.pat)
	if idx < 0 {
		return 0, 0, false
	}
	start = from + idx
	return start, start + len(m.pat), true
}

// regexMatcher wraps every other combination — RE2, so a user-typed pattern can never backtrack
// exponentially (§3.3's whole reason a regex box is safe to expose at all).
type regexMatcher struct{ re *regexp.Regexp }

func (m regexMatcher) find(line []byte, from int) (start, end int, ok bool) {
	if from > len(line) {
		return 0, 0, false
	}
	loc := m.re.FindIndex(line[from:])
	if loc == nil {
		return 0, 0, false
	}
	return from + loc[0], from + loc[1], true
}

// matchesOnLine returns up to max non-overlapping matches in line, in order.
//
// regexMatcher goes through a single re.FindAllIndex(line, max) call over the WHOLE line rather
// than scanFile's old find-and-reslice loop (repeated m.find(line[from:], 0) calls): Go's regexp
// evaluates ^, \A, and \b relative to the start of whatever byte slice it is handed, so re-slicing
// the line at each match's end presented a false "start of line"/"word boundary" to every
// subsequent match attempt on that line — `^import` against "importimport" wrongly reported two
// matches instead of one, and `bar|\bfoo` against "barfoo foo" wrongly reported a mid-word match at
// a `\b` alternative. FindAllIndex evaluates every match against the real, whole-line context.
//
// literalMatcher keeps its existing bytes.Index-based find-and-reslice fast path completely
// unchanged (bytes.Index has no anchor/boundary state to get wrong) — this loops through it here
// instead of inline in scanFile, purely to give both matcher kinds one call site.
func matchesOnLine(m matcher, line []byte, max int) [][]int {
	if max <= 0 {
		return nil
	}
	if rm, ok := m.(regexMatcher); ok {
		return rm.re.FindAllIndex(line, max)
	}
	locs := make([][]int, 0, 4)
	from := 0
	for len(locs) < max {
		start, end, ok := m.find(line, from)
		if !ok {
			break
		}
		locs = append(locs, []int{start, end})
		if end == start {
			from = start + 1 // zero-width advance — never loop forever on one line.
		} else {
			from = end
		}
	}
	return locs
}

// newMatcher builds the matcher: a plain literal query goes through bytes.Index; whole-word or
// case-insensitive or an explicit regex request goes through RE2, built by wrapping the (quoted or
// raw) pattern in \b(?:…)\b for whole-word and prefixing (?i) for case-insensitivity. A bad regex
// returns here, before any file is ever opened.
func newMatcher(req SearchRequest) (matcher, error) {
	if req.Query == "" {
		return nil, errors.New("query is required")
	}
	if !req.Regex && !req.WholeWord && req.CaseSensitive {
		return literalMatcher{pat: []byte(req.Query)}, nil
	}
	pattern := req.Query
	if !req.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if req.WholeWord {
		pattern = `\b(?:` + pattern + `)\b`
	}
	if !req.CaseSensitive {
		pattern = `(?i)` + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}
	return regexMatcher{re: re}, nil
}

// ValidatePattern compiles req's own pattern without running any search — StartSearch's own
// "compile before returning" step (§5), so a bad regex is E_BAD_REQUEST on the bound call itself,
// never a stream error the panel would have to render a second way.
func ValidatePattern(req SearchRequest) error {
	_, err := newMatcher(req)
	return err
}

// searchWorkers: a user-initiated foreground scan — min(NumCPU, 8), capped so a search never
// starves the UI thread's IPC.
func searchWorkers() int {
	if n := runtime.NumCPU(); n < 8 {
		if n < 1 {
			return 1
		}
		return n
	}
	return 8
}

// Search runs req against every file EnumerateAll reports for s.Root, streaming each file's own
// matches to onFile as they're found — callback-streamed, not slice-returning, so
// the bridge layer can coalesce without codeworkspace ever importing an emitter. onFile may
// be called concurrently with itself from different worker goroutines; the one caller
// (bridge's searchCoalescer) is documented to tolerate that with its own mutex.
//
// ctx should be the context returned by s.beginSearch() — Search itself derives an internal
// cancel from it so the whole-run cap (below) can stop outstanding work without cancelling the
// caller's own context.
func Search(ctx context.Context, s *Session, req SearchRequest, onFile func(FileMatches)) (SearchStats, error) {
	stats := SearchStats{}

	m, err := newMatcher(req)
	if err != nil {
		return stats, fmt.Errorf("codeworkspace: search: %w", err)
	}

	// One ls-files spawn per run: the listing is not cached from ListFiles because a tree loaded
	// minutes ago is not what a search should be answering against.
	paths, err := EnumerateAll(ctx, s.Runner, s.GitPath, s.Root)
	if err != nil {
		return stats, fmt.Errorf("codeworkspace: search: %w", err)
	}
	files, err := s.Files()
	if err != nil {
		return stats, fmt.Errorf("codeworkspace: search: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type outcome struct {
		path      string
		matches   []SearchMatch
		truncated bool
		skipped   bool
	}

	jobs := make(chan string)
	outcomes := make(chan outcome)

	var wg sync.WaitGroup
	workers := searchWorkers()
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// One reusable read buffer per worker (§3.4's own memory-bound note) — reused across
			// every file this worker scans, not reallocated per file.
			buf := make([]byte, 0, 64*1024)
			for path := range jobs {
				matches, truncated, skipped := scanFile(files, path, m, buf)
				select {
				case outcomes <- outcome{path: path, matches: matches, truncated: truncated, skipped: skipped}:
				case <-runCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(outcomes)
	}()
	go func() {
		defer close(jobs)
		for _, p := range paths {
			select {
			case jobs <- p:
			case <-runCtx.Done():
				return
			}
		}
	}()

	for o := range outcomes {
		stats.FilesScanned++
		if o.skipped {
			stats.FilesSkipped++
			continue
		}
		if len(o.matches) == 0 {
			continue
		}
		stats.FilesMatched++
		stats.Matches += len(o.matches)
		onFile(FileMatches{Path: o.path, Matches: o.matches, Truncated: o.truncated})

		if stats.Matches >= MaxSearchMatches && !stats.Truncated {
			// The whole-run cap: cancel the internal context so outstanding/future workers stop
			// starting new files, then keep draining outcomes already in flight (below) so no
			// worker goroutine leaks and no send on outcomes is left blocking.
			stats.Truncated = true
			cancel()
		}
	}

	if stats.Truncated {
		return stats, nil
	}
	return stats, ctx.Err()
}

// scanFile applies the five skip gates, in order, cheapest first, and the column/preview rules
// to one file. skipped reports whether this file counts toward FilesSkipped (any gate tripped,
// including gate 4's mid-scan NUL-on-a-match-line rule, which discards whatever matches were
// already collected for this file). buf is this worker's own reusable scan buffer.
func scanFile(files *fileservice.Service, relPath string, m matcher, buf []byte) (matches []SearchMatch, truncated bool, skipped bool) {
	// Gate 1 (D3) and gate 2: a path resolving outside root — including through a committed symlink —
	// a directory, a vanished file, a non-regular file, or one over MaxSearchFileBytes is skipped,
	// never read.
	//
	// F12: the regular-file check runs on this stat, before any open: stat cannot block whatever the
	// path resolves to, but opening a FIFO for reading blocks until another process opens its write
	// end, which would pin a search worker forever — files.go's ReadFile carries the identical fix.
	info, err := files.Stat(relPath)
	if err != nil || info.Dir || !info.Mode.IsRegular() || info.Size > MaxSearchFileBytes {
		return nil, false, true
	}

	f, _, err := files.Open(relPath)
	if err != nil {
		return nil, false, true
	}
	defer f.Close()

	// Gate 3: a NUL byte in the first 8 KiB marks the file binary — the identical rule ReadFile
	// already uses (Peek does not advance the reader, so the scanner below still sees the file from
	// byte 0).
	br := bufio.NewReaderSize(io.LimitReader(f, MaxSearchFileBytes+1), binarySniffBytes)
	peek, _ := br.Peek(binarySniffBytes)
	if bytes.IndexByte(peek, 0) >= 0 {
		return nil, false, true
	}

	scanner := bufio.NewScanner(br)
	scanner.Buffer(buf[:0], maxSearchLineBytes)

	line := 0
	for scanner.Scan() {
		line++
		// Rule 1 (textpos.go): a CRLF line's own '\r' is never part of Monaco's line content —
		// stripped before both matching and preview so a CRLF file never matches (or shows) it.
		raw := bytes.TrimSuffix(scanner.Bytes(), []byte("\r"))

		lineChecked := false
		for _, loc := range matchesOnLine(m, raw, MaxMatchesPerFile-len(matches)) {
			start, end := loc[0], loc[1]
			if !lineChecked {
				// Gate 4: checked once per matching line (never per line scanned, never per match
				// on the same line) — a NUL here means the file's body isn't the ASCII gate 3's
				// first-8-KiB peek suggested, so the whole file (including matches already
				// collected for earlier lines) is dropped.
				if bytes.IndexByte(raw, 0) >= 0 {
					return nil, false, true
				}
				lineChecked = true
			}

			preview, pmStart, pmEnd, truncStart, truncEnd := buildPreview(raw, start, end)
			matches = append(matches, SearchMatch{
				Line:              line,
				Column:            utf16Units(raw[:start]) + 1,
				EndColumn:         utf16Units(raw[:end]) + 1,
				Preview:           preview,
				PreviewMatchStart: pmStart,
				PreviewMatchEnd:   pmEnd,
				TruncatedStart:    truncStart,
				TruncatedEnd:      truncEnd,
			})
		}
		if len(matches) >= MaxMatchesPerFile {
			truncated = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		// Gate 5 (bufio.ErrTooLong) and any other mid-read failure both mean "skip this file whole"
		// — a partial result from a file that stopped scanning early would be misleading, not
		// merely incomplete.
		return nil, false, true
	}
	return matches, truncated, false
}

// buildPreview returns the preview text for a line already known to contain a match at the given
// byte span, plus that match's own UTF-16 offsets *into the preview*. A line at or under
// previewMaxBytes is shown whole; a longer one is windowed around the match, cut on rune
// boundaries so a copied preview is always real, valid file text.
func buildPreview(line []byte, start, end int) (preview string, matchStart, matchEnd int, truncStart, truncEnd bool) {
	if len(line) <= previewMaxBytes {
		return string(line), utf16Units(line[:start]), utf16Units(line[:end]), false, false
	}

	winStart := start - previewLeadBytes
	if winStart < 0 {
		winStart = 0
	}
	winStart = prevRuneBoundary(line, winStart)
	truncStart = winStart > 0

	winEnd := winStart + previewMaxBytes
	if winEnd > len(line) {
		winEnd = len(line)
	}
	winEnd = nextRuneBoundary(line, winEnd)
	truncEnd = winEnd < len(line)

	preview = string(line[winStart:winEnd])

	clampedEnd := end
	if clampedEnd > winEnd {
		clampedEnd = winEnd // a match whose own span exceeds the window clamps to the window's end.
	}
	matchStart = utf16Units(line[winStart:start])
	matchEnd = utf16Units(line[winStart:clampedEnd])
	return preview, matchStart, matchEnd, truncStart, truncEnd
}

// prevRuneBoundary returns the largest index <= i that starts a UTF-8 rune (or a single invalid
// byte, which utf8.RuneStart also treats as a boundary).
func prevRuneBoundary(b []byte, i int) int {
	for i > 0 && !utf8.RuneStart(b[i]) {
		i--
	}
	return i
}

// nextRuneBoundary returns the smallest index >= i that starts a UTF-8 rune, or len(b).
func nextRuneBoundary(b []byte, i int) int {
	for i < len(b) && !utf8.RuneStart(b[i]) {
		i++
	}
	return i
}
