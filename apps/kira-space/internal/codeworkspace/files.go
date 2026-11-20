package codeworkspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// MaxListedFiles is §7.1's own honest cap on a single IPC payload.
const MaxListedFiles = 200_000

// MaxReadBytes is §8.1's viewer cap — deliberately larger than C1's 2 MiB parse cap: showing a big
// file costs one allocation, parsing it costs symbols.
const MaxReadBytes = 8 * 1024 * 1024

// binarySniffBytes mirrors C1 §5.4's own rule: a NUL byte in the first 8 KiB marks a file binary.
const binarySniffBytes = 8 * 1024

// FileListing is §7.1's own wire shape — Paths as git reported them (repository-relative, never
// NFC-normalized), Status from one porcelain snapshot taken alongside the listing.
type FileListing struct {
	Paths     []string          `json:"paths"`
	Status    map[string]string `json:"status"`
	Truncated bool              `json:"truncated"`
}

// ListFiles lists every tracked-plus-untracked-but-not-ignored file in the session's root
// (EnumerateAll, enumerate.go) plus one `git status --porcelain=v2` snapshot collapsed to the
// tree's four-value glyph — one ls-files spawn, one status spawn, run sequentially (this is a
// side-panel refresh, not a hot path worth the concurrency Read/Write's own gate exists for).
func ListFiles(ctx context.Context, s *Session) (FileListing, error) {
	paths, err := EnumerateAll(ctx, s.Runner, s.GitPath, s.Root)
	if err != nil {
		return FileListing{}, fmt.Errorf("codeworkspace: list files: %w", err)
	}

	truncated := false
	if len(paths) > MaxListedFiles {
		paths = paths[:MaxListedFiles]
		truncated = true
	}

	status, err := statusOverlay(ctx, s)
	if err != nil {
		return FileListing{}, fmt.Errorf("codeworkspace: list files: %w", err)
	}

	return FileListing{Paths: paths, Status: status, Truncated: truncated}, nil
}

// statusOverlay runs one `git status --porcelain=v2 -z` and collapses every entry to the tree's
// own four-value status glyph, keyed by repository-relative path.
func statusOverlay(ctx context.Context, s *Session) (map[string]string, error) {
	args := porcelain.StatusArgs()
	res, err := gitclient.Run(ctx, s.Runner, s.GitPath, gitclient.Spec{Dir: s.Root, Args: args, ReadOnly: true})
	if cerr := gitclient.Classify(ctx, args, res, err); cerr != nil {
		return nil, fmt.Errorf("git status: %w", cerr)
	}
	splitter := porcelain.NewRecordSplitter(0)
	records, err := splitter.Push(res.Stdout)
	if err != nil {
		return nil, fmt.Errorf("split status: %w", err)
	}
	if flushed := splitter.Flush(); flushed != nil {
		return nil, fmt.Errorf("split status: unterminated trailing bytes: %q", flushed)
	}
	result, err := porcelain.ParseStatus(records)
	if err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}

	out := map[string]string{}
	for _, e := range result.Entries {
		switch e.Kind {
		case "untracked":
			out[e.Path] = "?"
		case "ordinary", "renamed":
			// XY collapsed to the tree's own four-value set: 'A'/'D' win over a plain 'M' when
			// either the staged or unstaged code says so, since either one is a real difference
			// from HEAD worth calling out distinctly from an ordinary edit.
			out[e.Path] = collapseXY(e.Staged, e.Unstaged)
		case "unmerged":
			// A conflict has no dedicated glyph in this four-value set (M/A/D/?) — 'M' is the
			// honest nearest value: real content differs from HEAD, which is what the tree's
			// coloring communicates either way.
			out[e.Path] = "M"
		}
	}
	return out, nil
}

func collapseXY(staged, unstaged byte) string {
	for _, c := range [2]byte{staged, unstaged} {
		switch c {
		case 'A':
			return "A"
		case 'D':
			return "D"
		}
	}
	return "M"
}

// FileContent is §8.1's own wire shape, mirroring the git contract's own `file.read` result shape
// (found/tooLarge) plus `missing` (a path git listed that no longer exists) and `language` (this
// app's own id, §9.4 — resolved from the extension only, never sniffed).
type FileContent struct {
	Kind       string `json:"kind"` // found | binary | tooLarge | missing | unchanged
	Text       string `json:"text"`
	Bytes      int    `json:"bytes"`
	LimitBytes int    `json:"limitBytes"`
	Language   string `json:"language"`
	// Stat identifies the bytes read; a caller passes it back as ifStat.
	Stat *FileStat `json:"stat,omitempty"`
}

// FileStat is the identity a caller keeps to skip resending an unchanged file: size plus the
// SHA-256 of the bytes read. Content is the only exact signal; mtime misses a same-size write inside
// the timestamp granularity and any tool that restores it.
type FileStat struct {
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

// ReadFile reads name through svc and classifies it per §8.1: a NUL byte in the first 8 KiB is binary
// (C1 §5.4's own rule), a file above MaxReadBytes is tooLarge, a missing file (deleted since
// ListFiles ran) reports `missing` rather than erroring the whole call. name is a worktree-relative
// path for a Files(root) Service, an absolute path for fileservice.Host().
//
// F12: the type check runs on svc.Stat, before any open: stat never blocks whatever name resolves to,
// open does (a FIFO with no writer hangs it). A symlink git tracked resolving to a FIFO (never an
// escape: Contain already rejects anything outside root) would otherwise pin the bound call or a
// search worker forever. The read itself is capped at MaxReadBytes: a regular file can still grow
// between the stat and the read, and the cap classifies tooLarge instead of loading it unbounded.
func ReadFile(svc *fileservice.Service, name string) (FileContent, error) {
	return ReadFileIf(svc, name, nil)
}

// ReadFileIf is ReadFile that answers kind "unchanged", with no content, when ifStat still matches
// the file on disk. It still reads and hashes the file: the gate saves the payload, not the read.
func ReadFileIf(svc *fileservice.Service, name string, ifStat *FileStat) (FileContent, error) {
	limit := FileContent{LimitBytes: MaxReadBytes, Language: languageFor(name)}

	info, err := svc.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		limit.Kind = "missing"
		return limit, nil
	}
	if err != nil {
		return FileContent{}, fmt.Errorf("codeworkspace: stat %s: %w", name, err)
	}
	if info.Dir || !info.Mode.IsRegular() {
		// A FIFO, device, socket, or anything else ListFiles' own git enumeration would never have
		// reported as a worktree file — treated the same as "gone", never opened.
		limit.Kind = "missing"
		return limit, nil
	}
	if info.Size > MaxReadBytes {
		limit.Kind = "tooLarge"
		limit.Bytes = int(info.Size)
		return limit, nil
	}

	data, _, err := svc.Read(name, MaxReadBytes)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		limit.Kind = "missing"
		return limit, nil
	case errors.Is(err, fileservice.ErrTooLarge):
		// Grew past the gate above between the stat and the read — info.Size is the best honest
		// answer left for Bytes (the exact final size is no longer meaningful).
		limit.Kind = "tooLarge"
		limit.Bytes = int(info.Size)
		return limit, nil
	case err != nil:
		return FileContent{}, fmt.Errorf("codeworkspace: read %s: %w", name, err)
	}

	sniffLen := len(data)
	if sniffLen > binarySniffBytes {
		sniffLen = binarySniffBytes
	}
	if strings.IndexByte(string(data[:sniffLen]), 0) >= 0 {
		limit.Kind = "binary"
		limit.Bytes = len(data)
		return limit, nil
	}

	limit.Stat = &FileStat{Size: int64(len(data)), Hash: hashBytes(data)}
	if ifStat != nil && *ifStat == *limit.Stat {
		limit.Kind = "unchanged"
		return limit, nil
	}
	limit.Kind = "found"
	limit.Bytes = len(data)
	limit.Text = string(data)
	return limit, nil
}

// languageByExt maps a lower-case extension to its Monaco language id. vue and svelte color with
// the HTML grammar (no Monaco grammar exists for either, §9.4).
var languageByExt = map[string]string{
	"ts":       "typescript",
	"tsx":      "typescript",
	"mts":      "typescript",
	"cts":      "typescript",
	"js":       "javascript",
	"jsx":      "javascript",
	"mjs":      "javascript",
	"cjs":      "javascript",
	"vue":      "html",
	"svelte":   "html",
	"json":     "json",
	"jsonc":    "json",
	"java":     "java",
	"py":       "python",
	"go":       "go",
	"rs":       "rust",
	"html":     "html",
	"htm":      "html",
	"css":      "css",
	"scss":     "scss",
	"less":     "less",
	"md":       "markdown",
	"markdown": "markdown",
	"yaml":     "yaml",
	"yml":      "yaml",
	"xml":      "xml",
	"sh":       "shell",
	"bash":     "shell",
	"zsh":      "shell",
	"sql":      "sql",
	"ini":      "ini",
	"cfg":      "ini",
	"toml":     "ini",
	"graphql":  "graphql",
	"gql":      "graphql",
	"proto":    "protobuf",
	"kt":       "kotlin",
	"kts":      "kotlin",
	"swift":    "swift",
	"c":        "c",
	"h":        "c",
	"cc":       "cpp",
	"cpp":      "cpp",
	"cxx":      "cpp",
	"hpp":      "cpp",
	"hh":       "cpp",
	"hxx":      "cpp",
	"m":        "objective-c",
	"mm":       "objective-c",
	"rb":       "ruby",
	"php":      "php",
	"pl":       "perl",
	"pm":       "perl",
	"lua":      "lua",
}

// languageFor resolves this app's own language id from relPath's extension only ("never sniffed")
// — the exact vocabulary registered as a Monarch contribution, plus 'plaintext' for
// everything else. Independent of views/repo/language.ts (frontend-side, by design — "different
// sets for different purposes... pretending otherwise would drag one to the other's shape");
// this is a second, intentionally small copy that exists only to answer this one wire field.
func languageFor(relPath string) string {
	if strings.ToLower(filepath.Base(relPath)) == "dockerfile" {
		return "dockerfile"
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(relPath), "."))
	if lang, ok := languageByExt[ext]; ok {
		return lang
	}
	return "plaintext"
}
