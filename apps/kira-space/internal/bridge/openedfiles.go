package bridge

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/openedfiles"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// OpenedFilesService is the page's side of an external open: it learns whether its window is the
// files window, pulls the files waiting for it and reads standalone ones. Open and Add stay off the bound surface.
type OpenedFilesService struct {
	Files *openedfiles.Service
	// Cwd is a terminal's live working directory; false when no live terminal has the id. A field,
	// not a method, so it stays off the bound surface.
	Cwd func(terminalID string) (string, bool)
}

// Limits for the terminal calls; maxTermPaths is the frontend parser's per-row cap.
const (
	maxTermPaths    = 20
	maxTermPathLen  = 4096
	maxTermPosition = 10_000_000
)

type OpenedFilesTerminalResolveArgs struct {
	TerminalID  string   `json:"terminalId"`
	FallbackCwd string   `json:"fallbackCwd"`
	Paths       []string `json:"paths"`
}

type OpenedFilesTerminalResolveResult struct {
	Openable []bool `json:"openable"`
	// Paths holds the absolute path per input, "" when not openable.
	Paths []string `json:"paths"`
}

type OpenedFilesTerminalOpenArgs struct {
	TerminalID  string `json:"terminalId"`
	FallbackCwd string `json:"fallbackCwd"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Col         int    `json:"col"`
}

type OpenedFilesPendingArgs struct {
	WindowKey string `json:"windowKey"`
}

type OpenedFilesReadArgs struct {
	Path string `json:"path"`
}

type OpenedFilesWindowArgs struct {
	WindowKey string `json:"windowKey"`
}

// IsFilesWindow reports whether windowKey names the files window; the page reads it once at boot.
func (s *OpenedFilesService) IsFilesWindow(args OpenedFilesWindowArgs) (bool, error) {
	if args.WindowKey == "" {
		return false, ipcerr.Required("windowKey")
	}
	return openedfiles.IsWindowKey(args.WindowKey), nil
}

// Pending returns the files windowKey was opened for, once.
func (s *OpenedFilesService) Pending(args OpenedFilesPendingArgs) ([]openedfiles.Target, error) {
	if args.WindowKey == "" {
		return nil, ipcerr.Required("windowKey")
	}
	if t := s.Files.Pending(args.WindowKey); t != nil {
		return t, nil
	}
	return []openedfiles.Target{}, nil
}

// Read reads one standalone file.
func (s *OpenedFilesService) Read(args OpenedFilesReadArgs) (codeworkspace.FileContent, error) {
	if args.Path == "" {
		return codeworkspace.FileContent{}, ipcerr.Required("path")
	}
	content, err := s.Files.Read(args.Path)
	if err != nil {
		return codeworkspace.FileContent{}, ipcerr.Contract("%s", err.Error())
	}
	return content, nil
}

// TerminalResolve reports per path whether it names a file Kira Space can open, against the
// terminal's live cwd, then fallbackCwd (the start cwd, also used once the terminal exited).
//
// Trust: as Read's comment says, bound calls come only from this app's webviews, which already
// run shells; the answer is a boolean limited to the supported file types.
func (s *OpenedFilesService) TerminalResolve(args OpenedFilesTerminalResolveArgs) (OpenedFilesTerminalResolveResult, error) {
	if args.TerminalID == "" {
		return OpenedFilesTerminalResolveResult{}, ipcerr.Contract("terminalId is required")
	}
	if n := len(args.Paths); n < 1 || n > maxTermPaths {
		return OpenedFilesTerminalResolveResult{}, ipcerr.Contract("paths must hold 1 to 20 entries")
	}
	for _, p := range args.Paths {
		if err := checkTermPath(p); err != nil {
			return OpenedFilesTerminalResolveResult{}, err
		}
	}
	cwds, err := s.termCwds(args.TerminalID, args.FallbackCwd)
	if err != nil {
		return OpenedFilesTerminalResolveResult{}, err
	}
	paths := s.Files.Openable(cwds, userHome(), args.Paths)
	openable := make([]bool, len(paths))
	for i, p := range paths {
		openable[i] = p != ""
	}
	return OpenedFilesTerminalResolveResult{Openable: openable, Paths: paths}, nil
}

// TerminalOpen opens one terminal path in an opened-files window, cursor at line and col.
func (s *OpenedFilesService) TerminalOpen(args OpenedFilesTerminalOpenArgs) error {
	if args.TerminalID == "" {
		return ipcerr.Contract("terminalId is required")
	}
	if err := checkTermPath(args.Path); err != nil {
		return err
	}
	if args.Line < 0 || args.Line > maxTermPosition || args.Col < 0 || args.Col > maxTermPosition {
		return ipcerr.Contract("line and col must be 0 to 10000000")
	}
	if args.Col > 0 && args.Line == 0 {
		return ipcerr.Contract("col needs line")
	}
	cwds, err := s.termCwds(args.TerminalID, args.FallbackCwd)
	if err != nil {
		return err
	}
	err = s.Files.OpenTerminalPath(args.Path, cwds, userHome(), args.Line, args.Col)
	if errors.Is(err, openedfiles.ErrNotOpenable) {
		return ipcerr.T(ipcerr.CodeNotFound, "backend.space.openedFiles.notOpenable.body", map[string]string{"path": args.Path})
	}
	return err
}

func checkTermPath(p string) error {
	if p == "" || len(p) > maxTermPathLen || strings.ContainsRune(p, 0) {
		return ipcerr.Contract("path must be 1 to 4096 bytes without NUL")
	}
	return nil
}

// termCwds lists the cwds to try: the terminal's live cwd, then fallback (the start cwd) when it
// differs. fallback is empty or absolute.
func (s *OpenedFilesService) termCwds(terminalID, fallback string) ([]string, error) {
	if fallback != "" && !filepath.IsAbs(fallback) {
		return nil, ipcerr.Contract("fallbackCwd must be absolute")
	}
	var out []string
	if s.Cwd != nil {
		if cwd, ok := s.Cwd(terminalID); ok && cwd != "" {
			out = append(out, cwd)
		}
	}
	if fallback != "" && !slices.Contains(out, fallback) {
		out = append(out, fallback)
	}
	return out, nil
}

func userHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
