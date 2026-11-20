package codeworkspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/catfile"
)

// DiffSide is one side of a HEAD-vs-worktree diff (C6 §5) — the same four-value classification
// FileContent already gives the worktree side, so a caller sees one consistent vocabulary for
// "what happened to this file" everywhere in the app.
type DiffSide struct {
	Kind       string `json:"kind"` // found | binary | tooLarge | missing | unchanged
	Text       string `json:"text"`
	Bytes      int    `json:"bytes"`
	LimitBytes int    `json:"limitBytes"`
	// Stat identifies the worktree side; Oid identifies the HEAD side's blob. A caller passes them
	// back as DiffGate to skip re-reading a side that has not changed.
	Stat *FileStat `json:"stat,omitempty"`
	Oid  string    `json:"oid,omitempty"`
}

// DiffGate carries what a caller already holds. A side that still matches answers kind
// "unchanged" with no text.
type DiffGate struct {
	WorktreeStat *FileStat
	HeadOid      string
}

// DiffContent is ReadDiff's own wire shape — no status field (D12): the two sides already say what
// happened (HEAD missing means added, worktree missing means deleted, both present means
// modified), so a second `git status` spawn to restate that would be cost for nothing.
type DiffContent struct {
	Path     string   `json:"path"`
	Language string   `json:"language"`
	Head     DiffSide `json:"head"`
	Worktree DiffSide `json:"worktree"`
	// ContentHash identifies Worktree; set only by the uncommitted review read, which echoes it on
	// a mark or comment so the server can refuse one built on content the user never saw.
	ContentHash string `json:"contentHash,omitempty"`
}

// ReadDiff reads relPath's own HEAD and worktree content for a diff tab (C6 §5) — validating
// relPath against the session's own root itself (§11's boundary), the one path-safety check every
// other read primitive already performs before ever touching disk.
func ReadDiff(ctx context.Context, s *Session, relPath string) (DiffContent, error) {
	return ReadDiffGated(ctx, s, relPath, DiffGate{})
}

// ReadDiffGated is ReadDiff that skips a side gate still matches.
func ReadDiffGated(ctx context.Context, s *Session, relPath string, gate DiffGate) (DiffContent, error) {
	cf, err := s.catfileSession()
	if err != nil {
		return DiffContent{}, fmt.Errorf("codeworkspace: read diff HEAD side: %w", err)
	}
	return readDiffAt(ctx, cf, s.Root, "HEAD", relPath, gate)
}

// ReadDiffAt is ReadDiff over an explicit root and revision: relPath as it stood at headRev (a
// commit-ish; "" means no original) against the file under root. A linked worktree passes its own
// tip, since "HEAD" through a shared cat-file session resolves the main checkout's HEAD.
func ReadDiffAt(ctx context.Context, cf *catfile.Session, root, headRev, relPath string) (DiffContent, error) {
	return readDiffAt(ctx, cf, root, headRev, relPath, DiffGate{})
}

func readDiffAt(ctx context.Context, cf *catfile.Session, root, headRev, relPath string, gate DiffGate) (DiffContent, error) {
	files, err := Files(root)
	if err != nil {
		return DiffContent{}, fmt.Errorf("codeworkspace: read diff: %w", err)
	}
	if _, err := files.Path(relPath); err != nil {
		return DiffContent{}, fmt.Errorf("codeworkspace: read diff: %w", err)
	}

	worktree, err := ReadFileIf(files, relPath, gate.WorktreeStat)
	if err != nil {
		return DiffContent{}, fmt.Errorf("codeworkspace: read diff worktree side: %w", err)
	}

	head := DiffSide{Kind: "missing", LimitBytes: MaxReadBytes}
	if headRev != "" {
		head, err = readHeadSide(ctx, cf, headRev+":"+relPath, gate.HeadOid)
		if err != nil {
			return DiffContent{}, fmt.Errorf("codeworkspace: read diff HEAD side: %w", err)
		}
	}

	return DiffContent{
		Path:     relPath,
		Language: languageFor(relPath),
		Head:     head,
		Worktree: DiffSide{Kind: worktree.Kind, Text: worktree.Text, Bytes: worktree.Bytes, LimitBytes: worktree.LimitBytes, Stat: worktree.Stat},
	}, nil
}

// readHeadSide reads relPath as it stood at HEAD via the session's own lazily-constructed
// catfile.Session — the same 8 MiB gate as the worktree side (MaxReadBytes), not catfile's own
// larger default, so the two sides can never disagree about what is too large.
func readHeadSide(ctx context.Context, cf *catfile.Session, rev, ifOid string) (DiffSide, error) {
	var info catfile.ObjectInfo
	var data []byte
	var err error
	// --batch-check reads no content; a matching blob id means the caller's copy is current.
	if ifOid != "" && !strings.Contains(rev, "\n") {
		if known, cerr := cf.Check(ctx, rev); cerr == nil && known.OID == ifOid {
			return DiffSide{Kind: "unchanged", Oid: ifOid, LimitBytes: MaxReadBytes}, nil
		}
	}
	if strings.Contains(rev, "\n") {
		// gitsession.Blob's own guard, carried here verbatim: a newline anywhere in the request
		// desynchronises the one-line-in/one-line-out batch protocol for the life of the session,
		// so a path containing one must go through the one-shot, argv-only path instead.
		info, data, err = cf.ReadOneShot(ctx, rev)
	} else {
		info, data, err = cf.Read(ctx, rev)
	}

	switch {
	case errors.Is(err, catfile.ErrMissing):
		// A new file, an untracked file, or an unborn HEAD — all "no original" from the diff's
		// own point of view.
		return DiffSide{Kind: "missing", LimitBytes: MaxReadBytes}, nil
	case errors.Is(err, catfile.ErrTooLarge):
		return DiffSide{Kind: "tooLarge", LimitBytes: MaxReadBytes}, nil
	case err != nil:
		return DiffSide{}, err
	}

	sniffLen := len(data)
	if sniffLen > binarySniffBytes {
		sniffLen = binarySniffBytes
	}
	if strings.IndexByte(string(data[:sniffLen]), 0) >= 0 {
		return DiffSide{Kind: "binary", Bytes: int(info.Size), LimitBytes: MaxReadBytes, Oid: info.OID}, nil
	}

	return DiffSide{Kind: "found", Text: string(data), Bytes: int(info.Size), LimitBytes: MaxReadBytes, Oid: info.OID}, nil
}
