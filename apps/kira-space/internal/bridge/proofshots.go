package bridge

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

const (
	maxShotFolders  = 8
	maxFolderSuffix = 99
)

// shotFolder is a folder ProofService created. A closed folder takes no more saves but can still be revealed.
type shotFolder struct {
	path   string
	closed bool
}

// proofShotFolders maps an opaque id to a folder ProofService created, so the renderer never sends a path.
type proofShotFolders struct {
	mu      sync.Mutex
	folders map[string]*shotFolder
	order   []string
}

func (p *proofShotFolders) add(path string) string {
	id := ids.New(ids.Shot)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.folders == nil {
		p.folders = map[string]*shotFolder{}
	}
	p.folders[id] = &shotFolder{path: path}
	p.order = append(p.order, id)
	if len(p.order) > maxShotFolders {
		delete(p.folders, p.order[0])
		p.order = p.order[1:]
	}
	return id
}

func (p *proofShotFolders) get(id string) (shotFolder, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.folders[id]
	if !ok {
		return shotFolder{}, false
	}
	return *f, true
}

func (p *proofShotFolders) close(id string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.folders[id]
	if !ok {
		return "", false
	}
	f.closed = true
	return f.path, true
}

// ProofShotLocation is the parent folder screenshot folders are created in.
type ProofShotLocation struct {
	Path   string `json:"path"`   // effective parent folder
	Custom bool   `json:"custom"` // false: the Desktop default
	Exists bool   `json:"exists"` // false: the custom folder is gone
}

// ProofChooseShotLocationResult is the outcome of the location picker; canceled leaves the location unchanged.
type ProofChooseShotLocationResult struct {
	Canceled bool              `json:"canceled"`
	Location ProofShotLocation `json:"location"`
}

// ProofOpenShotFolderArgs names the session whose tabs are about to be saved.
type ProofOpenShotFolderArgs struct {
	SessionName string `json:"sessionName"`
}

// ProofShotFolderResult is the fresh folder screenshots go into.
type ProofShotFolderResult struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// ProofSaveJpegToArgs saves one screenshot into a folder from OpenShotFolder.
type ProofSaveJpegToArgs struct {
	FolderID   string `json:"folderId"`
	Filename   string `json:"filename"`
	JpegBase64 string `json:"jpegBase64"`
}

// ProofSaveJpegToResult is the written file.
type ProofSaveJpegToResult struct {
	Path string `json:"path"`
}

// ProofShotFolderArgs names a folder from OpenShotFolder.
type ProofShotFolderArgs struct {
	FolderID string `json:"folderId"`
}

// desktopDir resolves the default parent on every call, so it follows the current HOME. Never creates ~/Desktop.
func desktopDir() (string, error) {
	if d := os.Getenv("XDG_DESKTOP_DIR"); filepath.IsAbs(d) && isDir(d) {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ipcerr.InternalErr(err)
	}
	if d := filepath.Join(home, "Desktop"); isDir(d) {
		return d, nil
	}
	return home, nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (s *ProofService) shotLocation() (ProofShotLocation, error) {
	custom, err := s.Locations.ProofShotLocation()
	if err != nil {
		return ProofShotLocation{}, ipcerr.InternalErr(err)
	}
	if custom != "" {
		return ProofShotLocation{Path: custom, Custom: true, Exists: isDir(custom)}, nil
	}
	path, err := desktopDir()
	if err != nil {
		return ProofShotLocation{}, err
	}
	return ProofShotLocation{Path: path, Exists: true}, nil
}

// ShotLocation returns the parent folder screenshots are saved under. It reads the store every call.
func (s *ProofService) ShotLocation() (ProofShotLocation, error) { return s.shotLocation() }

// ChooseShotLocation asks for a new parent folder and stores it. Cancel and an unusable pick change nothing.
func (s *ProofService) ChooseShotLocation() (ProofChooseShotLocationResult, error) {
	picked, err := s.Dialogs.OpenDirectory(OpenDirectoryRequest{Title: i18n.T("backend.studio.proof.shotLocation.title", nil)})
	if err != nil {
		return ProofChooseShotLocationResult{}, ipcerr.InternalErr(err)
	}
	if picked == "" {
		loc, err := s.shotLocation()
		return ProofChooseShotLocationResult{Canceled: true, Location: loc}, err
	}
	picked = filepath.Clean(picked)
	if err := checkWritableDir(picked); err != nil {
		return ProofChooseShotLocationResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotLocationInvalid.body", map[string]string{"path": picked})
	}
	if err := s.Locations.SetProofShotLocation(picked); err != nil {
		return ProofChooseShotLocationResult{}, ipcerr.InternalErr(err)
	}
	loc, err := s.shotLocation()
	return ProofChooseShotLocationResult{Location: loc}, err
}

// checkWritableDir requires an absolute, existing, writable directory.
func checkWritableDir(path string) error {
	if !filepath.IsAbs(path) || !isDir(path) {
		return os.ErrInvalid
	}
	probe, err := os.MkdirTemp(path, ".kira-shot-*")
	if err != nil {
		return err
	}
	return os.Remove(probe)
}

// ResetShotLocation drops the custom folder, back to the Desktop default.
func (s *ProofService) ResetShotLocation() (ProofShotLocation, error) {
	if err := s.Locations.SetProofShotLocation(""); err != nil {
		return ProofShotLocation{}, ipcerr.InternalErr(err)
	}
	return s.shotLocation()
}

// OpenShotFolder creates a fresh folder named after the session under the save location. No picker.
func (s *ProofService) OpenShotFolder(args ProofOpenShotFolderArgs) (ProofShotFolderResult, error) {
	loc, err := s.shotLocation()
	if err != nil {
		return ProofShotFolderResult{}, err
	}
	if loc.Custom && !loc.Exists {
		return ProofShotFolderResult{}, missingLocationErr(loc.Path)
	}
	path, err := mkFreshDir(loc.Path, safeBaseName(args.SessionName))
	if err != nil {
		return ProofShotFolderResult{}, err
	}
	return ProofShotFolderResult{ID: s.shots.add(path), Path: path}, nil
}

func missingLocationErr(path string) error {
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotLocationMissing.body", map[string]string{"path": path})
}

// mkFreshDir creates parent/name, or parent/name-2 ... when taken. Mkdir, never MkdirAll, so nothing is reused.
func mkFreshDir(parent, name string) (string, error) {
	for i := 1; i <= maxFolderSuffix; i++ {
		p := filepath.Join(parent, name)
		if i > 1 {
			p = filepath.Join(parent, fmt.Sprintf("%s-%d", name, i))
		}
		err := os.Mkdir(p, 0o755)
		if err == nil {
			return p, nil
		}
		switch {
		case errors.Is(err, os.ErrExist):
		case errors.Is(err, os.ErrNotExist):
			return "", missingLocationErr(parent)
		case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EROFS):
			return "", ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotLocationUnwritable.body", map[string]string{"path": parent})
		default:
			return "", ipcerr.InternalErr(err)
		}
	}
	return "", ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotFolderTaken.body", map[string]string{"name": name, "path": parent})
}

// SaveJpegTo writes one screenshot into a folder from OpenShotFolder, never over an existing file.
func (s *ProofService) SaveJpegTo(args ProofSaveJpegToArgs) (ProofSaveJpegToResult, error) {
	folder, ok := s.shots.get(args.FolderID)
	if !ok || folder.closed {
		return ProofSaveJpegToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotFolderGone.body", nil)
	}
	dir := folder.path
	if len(args.JpegBase64) > maxShotBase64 {
		return ProofSaveJpegToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.tooLarge.body", nil)
	}
	data, err := base64.StdEncoding.DecodeString(args.JpegBase64)
	if err != nil || len(data) > maxShotBytes || !bytes.HasPrefix(data, jpegMagic) {
		return ProofSaveJpegToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.notJpeg.body", nil)
	}
	path := filepath.Join(dir, safeJpegName(args.Filename))
	if _, err := os.Lstat(path); err == nil {
		return ProofSaveJpegToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotExists.body", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProofSaveJpegToResult{}, ipcerr.InternalErr(err)
	}
	if err := renameio.WriteFile(path, data, 0o644); err != nil {
		return ProofSaveJpegToResult{}, ipcerr.InternalErr(err)
	}
	return ProofSaveJpegToResult{Path: path}, nil
}

// CloseShotFolder ends a save walk: no more saves for the id, and the folder is removed when nothing was written.
func (s *ProofService) CloseShotFolder(args ProofShotFolderArgs) error {
	if path, ok := s.shots.close(args.FolderID); ok {
		// os.Remove only succeeds on an empty directory.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Debug("proof: keep shot folder", "scope", "proof", "path", path, "err", err)
		}
	}
	return nil
}

// RevealShotFolder shows a saved folder in the file manager.
func (s *ProofService) RevealShotFolder(args ProofShotFolderArgs) error {
	if s.Opener == nil {
		return ipcerr.T(ipcerr.CodeUnavailable, "backend.studio.proof.unavailable.body", nil)
	}
	folder, ok := s.shots.get(args.FolderID)
	if !ok || !isDir(folder.path) {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotFolderGone.body", nil)
	}
	if err := s.Opener.RevealFile(folder.path); err != nil {
		return ipcerr.InternalErr(err)
	}
	return nil
}
