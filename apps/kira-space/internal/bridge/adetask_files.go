package bridge

import (
	"context"
	"errors"
	"runtime"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/editorlaunch"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// adetask_files.go: the file view of a task branch's worktree, read-only, plus Reveal and Open in
// VS Code. The client sends a branch id and a relative path; the worktree root comes from the
// branch row and every path is contained under it (ade.TaskFileTarget, fileservice Contain).

// EditorLauncher opens paths in the user's code editor, detached. Nil means this build has none.
type EditorLauncher interface {
	Open(paths []string) error
}

// TaskRevealer reveals a path in the platform file manager. Nil means this build cannot.
type TaskRevealer interface {
	RevealFile(path string) error
}

// AdeTaskFileArgs names one file of a branch's worktree; Path "" is the worktree root for Reveal
// and Open in editor.
type AdeTaskFileArgs struct {
	BranchID string `json:"branchId"`
	Path     string `json:"path"`
	// IfStat is optional: TaskFileRead answers kind "unchanged", with no content, for a file that
	// still matches it.
	IfStat *codeworkspace.FileStat `json:"ifStat,omitempty"`
}

// AdeTaskFilesSearchArgs starts the branch worktree's content search; WindowKey addresses the
// coalesced push at the one window that asked.
type AdeTaskFilesSearchArgs struct {
	BranchID      string `json:"branchId"`
	WindowKey     string `json:"windowKey"`
	Query         string `json:"query"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
}

// AdeTaskLaunchers says what Reveal and Open in editor do in this build. Reveal is "finder",
// "explorer", "fileManager" or "none"; Editor is false when no editor can be launched.
type AdeTaskLaunchers struct {
	Reveal string `json:"reveal"`
	Editor bool   `json:"editor"`
}

// AdeTaskFileListing is the branch's file listing with the root it lists; Main is true when the
// row has no worktree and the repo's main worktree shows instead.
type AdeTaskFileListing struct {
	Paths     []string          `json:"paths"`
	Status    map[string]string `json:"status"`
	Truncated bool              `json:"truncated"`
	Root      string            `json:"root"`
	Main      bool              `json:"main"`
}

// TaskFiles lists the files of the branch's worktree, or its repo's main worktree.
func (s *AdeTaskService) TaskFiles(ctx context.Context, args adewire.BranchArgs) (AdeTaskFileListing, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return AdeTaskFileListing{}, err
	}
	l, err := s.Engine.TaskFiles(ctx, args.BranchID)
	if err != nil {
		return AdeTaskFileListing{}, adeTaskError(err)
	}
	return AdeTaskFileListing{Paths: l.Paths, Status: l.Status, Truncated: l.Truncated, Root: l.Root, Main: l.Main}, nil
}

// TaskFileRead reads one worktree file.
func (s *AdeTaskService) TaskFileRead(ctx context.Context, args AdeTaskFileArgs) (codeworkspace.FileContent, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return codeworkspace.FileContent{}, err
	}
	if args.Path == "" {
		return codeworkspace.FileContent{}, ipcerr.Required("path")
	}
	c, err := s.Engine.TaskFileRead(ctx, args.BranchID, args.Path, args.IfStat)
	return c, adeTaskError(err)
}

// TaskFilesSearch starts the branch worktree's one search; results stream on kira:code:search.
func (s *AdeTaskService) TaskFilesSearch(ctx context.Context, args AdeTaskFilesSearchArgs) (CodeWorkspaceSearchHandle, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return CodeWorkspaceSearchHandle{}, err
	}
	if args.WindowKey == "" {
		return CodeWorkspaceSearchHandle{}, ipcerr.Required("windowKey")
	}
	if args.Query == "" {
		return CodeWorkspaceSearchHandle{}, ipcerr.Required("query")
	}
	req := codeworkspace.SearchRequest{
		Query: args.Query, Regex: args.Regex, CaseSensitive: args.CaseSensitive, WholeWord: args.WholeWord,
	}
	if err := codeworkspace.ValidatePattern(req); err != nil {
		return CodeWorkspaceSearchHandle{}, ipcerr.BadRequest(err.Error())
	}
	sess, err := s.Engine.TaskFilesSession(ctx, args.BranchID)
	if err != nil {
		return CodeWorkspaceSearchHandle{}, adeTaskError(err)
	}
	return runSearch(s.Emit, sess, args.WindowKey, req), nil
}

// TaskFilesCancelSearch stops the branch's in-flight search. It opens no session.
func (s *AdeTaskService) TaskFilesCancelSearch(args adewire.BranchArgs) error {
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	if sess := s.Engine.TaskFilesPeek(args.BranchID); sess != nil {
		sess.CancelSearch()
	}
	return nil
}

// TaskFilesClose drops the branch's file session; the UI calls it when the Files tab goes away.
func (s *AdeTaskService) TaskFilesClose(args adewire.BranchArgs) error {
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	s.Engine.TaskFilesClose(args.BranchID)
	return nil
}

// TaskFileReveal reveals a worktree file (selected) or the worktree root in the file manager.
func (s *AdeTaskService) TaskFileReveal(ctx context.Context, args AdeTaskFileArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	if s.Revealer == nil {
		return ipcerr.T(ipcerr.CodeUnsupported, "backend.space.ade.files.launchUnsupported.body", nil)
	}
	_, abs, err := s.Engine.TaskFileTarget(ctx, args.BranchID, args.Path)
	if err != nil {
		return adeTaskError(err)
	}
	return ipcerr.InternalErr(s.Revealer.RevealFile(abs))
}

// TaskFileOpenInEditor opens the worktree (and the file, when Path is set) in VS Code.
func (s *AdeTaskService) TaskFileOpenInEditor(ctx context.Context, args AdeTaskFileArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	if s.Editor == nil {
		return ipcerr.T(ipcerr.CodeUnsupported, "backend.space.ade.files.launchUnsupported.body", nil)
	}
	root, abs, err := s.Engine.TaskFileTarget(ctx, args.BranchID, args.Path)
	if err != nil {
		return adeTaskError(err)
	}
	paths := []string{root}
	if args.Path != "" {
		paths = append(paths, abs)
	}
	if err := s.Editor.Open(paths); err != nil {
		if errors.Is(err, editorlaunch.ErrNotFound) {
			return ipcerr.T(ipcerr.CodeNotFound, "backend.space.ade.files.editorMissing.body", nil)
		}
		return ipcerr.InternalErr(err)
	}
	return nil
}

// TaskFilesLaunchers reports which of Reveal and Open in editor this build offers.
func (s *AdeTaskService) TaskFilesLaunchers() AdeTaskLaunchers {
	l := AdeTaskLaunchers{Reveal: "none", Editor: s.Editor != nil}
	if s.Revealer == nil {
		return l
	}
	switch runtime.GOOS {
	case "darwin":
		l.Reveal = "finder"
	case "windows":
		l.Reveal = "explorer"
	default:
		l.Reveal = "fileManager"
	}
	return l
}
