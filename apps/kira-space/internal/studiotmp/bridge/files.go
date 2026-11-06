package bridge

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// The four wire shapes, byte for byte packages/shared/protocol/ipc.ts:133-149's.
type FilesChooseSaveArgs struct {
	DefaultName string `json:"defaultName"`
}
type FilesChooseSaveResult struct {
	Canceled bool    `json:"canceled"`
	FilePath *string `json:"filePath"`
}

type FileFilter struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
}
type FilesChooseOpenArgs struct {
	Filters []FileFilter `json:"filters,omitempty"`
	Title   string       `json:"title,omitempty"`
}
type ChosenFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
type FilesChooseOpenManyResult struct {
	Canceled bool         `json:"canceled"`
	Files    []ChosenFile `json:"files"`
}
type FilesChooseOpenResult struct {
	Canceled bool        `json:"canceled"`
	File     *ChosenFile `json:"file"`
}

// SaveFileRequest / OpenFileRequest are the platform-neutral asks. FilterPattern is already in
// Wails' own "*.a;*.b" form (dialogs.go:279's AddFilter doc); an empty FilterName means "no
// filter", which is how D8's `*` case is expressed.
type SaveFileRequest struct{ Directory, Filename string }
type OpenFileRequest struct{ Title, FilterName, FilterPattern string }

// OpenFilesRequest is OpenFileRequest for a multi-select panel.
type OpenFilesRequest struct{ Title, FilterName, FilterPattern string }

// OpenDirectoryRequest is P25 D13's folder picker — a "no filters" version of OpenFileRequest,
// its own type since a filter makes no sense for a directory chooser.
type OpenDirectoryRequest struct{ Title string }

// Dialogs is the native-dialog seam. internal/appshell adapts it over repo-root
// internal/shell.Dialogs (app.Dialog with the main window attached for modality);
// files_test.go implements it with a recorder. Every method returns "" for a cancelled dialog,
// which is the only cancel signal Wails gives.
type Dialogs interface {
	SaveFile(req SaveFileRequest) (string, error)
	OpenFile(req OpenFileRequest) (string, error)
	// OpenFiles returns no paths for a cancelled dialog.
	OpenFiles(req OpenFilesRequest) ([]string, error)
	// OpenDirectory is P25 D13: the same OpenFile panel with CanChooseFiles(false)/
	// CanChooseDirectories(true) (Wails v3 beta.16's OpenFileDialogStruct already supports this —
	// no new native mechanism).
	OpenDirectory(req OpenDirectoryRequest) (string, error)
}

type FilesService struct {
	Dialogs Dialogs
}

// ChooseSave ports src/main/ipc/files.ts:26-36. filepath.Base, not the suggested name verbatim —
// an S3 key routinely contains '/', which the save panel would otherwise read as a subdirectory
// path (P56 D9, and files.ts:29-30's own comment).
func (s *FilesService) ChooseSave(args FilesChooseSaveArgs) (FilesChooseSaveResult, error) {
	if args.DefaultName == "" {
		return FilesChooseSaveResult{}, ipcerr.Required("defaultName")
	}
	path, err := s.Dialogs.SaveFile(SaveFileRequest{
		Directory: downloadsDir(),
		Filename:  filepath.Base(args.DefaultName),
	})
	if err != nil {
		return FilesChooseSaveResult{}, ipcerr.InternalErr(err)
	}
	if path == "" {
		return FilesChooseSaveResult{Canceled: true}, nil
	}
	return FilesChooseSaveResult{Canceled: false, FilePath: &path}, nil
}

// ChooseOpen ports files.ts:38-53, including the stat() that fills `size`.
func (s *FilesService) ChooseOpen(args FilesChooseOpenArgs) (FilesChooseOpenResult, error) {
	req := OpenFileRequest{Title: args.Title}
	if name, pattern, ok := wailsFilter(args.Filters); ok {
		req.FilterName, req.FilterPattern = name, pattern
	}

	path, err := s.Dialogs.OpenFile(req)
	if err != nil {
		return FilesChooseOpenResult{}, ipcerr.InternalErr(err)
	}
	if path == "" {
		return FilesChooseOpenResult{Canceled: true}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return FilesChooseOpenResult{}, ipcerr.T(ipcerr.CodeInternal, "backend.studio.file.readFailed.error", map[string]string{"path": path, "detail": err.Error()})
	}
	return FilesChooseOpenResult{
		Canceled: false,
		File:     &ChosenFile{Path: path, Name: filepath.Base(path), Size: info.Size()},
	}, nil
}

// ChooseOpenMany is ChooseOpen with several selections allowed.
func (s *FilesService) ChooseOpenMany(args FilesChooseOpenArgs) (FilesChooseOpenManyResult, error) {
	req := OpenFilesRequest{Title: args.Title}
	if name, pattern, ok := wailsFilter(args.Filters); ok {
		req.FilterName, req.FilterPattern = name, pattern
	}
	paths, err := s.Dialogs.OpenFiles(req)
	if err != nil {
		return FilesChooseOpenManyResult{}, ipcerr.InternalErr(err)
	}
	if len(paths) == 0 {
		return FilesChooseOpenManyResult{Canceled: true}, nil
	}
	files := make([]ChosenFile, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return FilesChooseOpenManyResult{}, ipcerr.T(ipcerr.CodeInternal, "backend.studio.file.readFailed.error", map[string]string{"path": path, "detail": err.Error()})
		}
		files = append(files, ChosenFile{Path: path, Name: filepath.Base(path), Size: info.Size()})
	}
	return FilesChooseOpenManyResult{Files: files}, nil
}

// FilesChooseFolderArgs / FilesChooseFolderResult are P25 D13's folder picker — ChooseOpen's own
// "" means cancelled" convention (Path == nil, Canceled == true), the same shape ChooseSave uses.
type FilesChooseFolderArgs struct {
	Title string `json:"title,omitempty"`
}
type FilesChooseFolderResult struct {
	Canceled bool    `json:"canceled"`
	Path     *string `json:"path"`
}

// ChooseFolder is the picker for a DataGrip project directory. It also accepts a directly
// typed path (the dialog UI itself validates that path the same way, so this method stays a thin
// wrapper with no directory-shape validation of its own — Scan's own error is what a bad path
// surfaces as).
func (s *FilesService) ChooseFolder(args FilesChooseFolderArgs) (FilesChooseFolderResult, error) {
	path, canceled, err := shell.ChooseFolder(func(title string) (string, error) {
		return s.Dialogs.OpenDirectory(OpenDirectoryRequest{Title: title})
	}, args.Title)
	if err != nil {
		return FilesChooseFolderResult{}, err
	}
	if canceled {
		return FilesChooseFolderResult{Canceled: true}, nil
	}
	return FilesChooseFolderResult{Canceled: false, Path: &path}, nil
}

// wailsFilter collapses Electron's per-group filter list into the single extension set Wails'
// macOS open panel applies (dialogs_darwin.go's show() joins every filter's components
// into one ';' string). Returns ok == false when any extension is "*": Wails has no wildcard —
// panel:shouldEnableURL: matches on a literal ".<ext>" suffix (dialogs_darwin_delegate.m:29-37) —
// and an empty allowed-extension list is what means "all files" there.
func wailsFilter(filters []FileFilter) (name, pattern string, ok bool) {
	if len(filters) == 0 {
		return "", "", false
	}
	var patterns []string
	for _, f := range filters {
		for _, ext := range f.Extensions {
			if ext == "*" {
				return "", "", false
			}
			patterns = append(patterns, "*."+ext)
		}
	}
	if len(patterns) == 0 {
		return "", "", false
	}
	return filters[0].Name, strings.Join(patterns, ";"), true
}

// downloadsDir is app.getPath('downloads')'s substitute; Wails' EnvironmentManager exposes no
// path API at all (environment_manager.go:23-58).
func downloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Downloads")
}
