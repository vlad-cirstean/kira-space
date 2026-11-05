package bridge

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shortid"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

const (
	maxShotFolders  = 8
	maxFolderSuffix = 99
)

// proofShotFolders maps an opaque id to a folder the user picked, so the renderer never sends a path.
type proofShotFolders struct {
	mu    sync.Mutex
	paths map[string]string
	order []string
}

func (p *proofShotFolders) add(path string) (string, error) {
	n, err := shortid.New(12)
	if err != nil {
		return "", err
	}
	id := "shot_" + n
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.paths == nil {
		p.paths = map[string]string{}
	}
	p.paths[id] = path
	p.order = append(p.order, id)
	if len(p.order) > maxShotFolders {
		delete(p.paths, p.order[0])
		p.order = p.order[1:]
	}
	return id, nil
}

func (p *proofShotFolders) get(id string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	path, ok := p.paths[id]
	return path, ok
}

func (p *proofShotFolders) forget(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.paths, id)
	for i, o := range p.order {
		if o == id {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
}

// ProofChooseShotFolderArgs names the session whose tabs are about to be saved.
type ProofChooseShotFolderArgs struct {
	SessionName string `json:"sessionName"`
}

// ProofShotFolderResult is the fresh folder screenshots go into; canceled when the user closed the picker.
type ProofShotFolderResult struct {
	Canceled bool   `json:"canceled"`
	ID       string `json:"id"`
	Path     string `json:"path"`
}

// ProofSavePngToArgs saves one screenshot into a folder from ChooseShotFolder.
type ProofSavePngToArgs struct {
	FolderID  string `json:"folderId"`
	Filename  string `json:"filename"`
	PngBase64 string `json:"pngBase64"`
}

// ProofSavePngToResult is the written file.
type ProofSavePngToResult struct {
	Path string `json:"path"`
}

// ProofForgetShotFolderArgs releases a folder id.
type ProofForgetShotFolderArgs struct {
	FolderID string `json:"folderId"`
}

// ChooseShotFolder asks for a parent folder and creates a fresh subfolder named after the session.
func (s *ProofService) ChooseShotFolder(args ProofChooseShotFolderArgs) (ProofShotFolderResult, error) {
	parent, err := s.Dialogs.OpenDirectory(OpenDirectoryRequest{Title: i18n.T("backend.studio.proof.shotFolder.title", nil)})
	if err != nil {
		return ProofShotFolderResult{}, ipcerr.InternalErr(err)
	}
	if parent == "" {
		return ProofShotFolderResult{Canceled: true}, nil
	}
	path, err := mkFreshDir(parent, strings.TrimSuffix(safePngName(args.SessionName), ".png"))
	if err != nil {
		return ProofShotFolderResult{}, err
	}
	id, err := s.shots.add(path)
	if err != nil {
		return ProofShotFolderResult{}, ipcerr.InternalErr(err)
	}
	return ProofShotFolderResult{ID: id, Path: path}, nil
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
		if !errors.Is(err, os.ErrExist) {
			return "", ipcerr.InternalErr(err)
		}
	}
	return "", ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotFolderTaken.body", nil)
}

// SavePngTo writes one screenshot into a picked folder, never over an existing file.
func (s *ProofService) SavePngTo(args ProofSavePngToArgs) (ProofSavePngToResult, error) {
	dir, ok := s.shots.get(args.FolderID)
	if !ok {
		return ProofSavePngToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotFolderGone.body", nil)
	}
	if len(args.PngBase64) > maxPngBase64 {
		return ProofSavePngToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.tooLarge.body", nil)
	}
	data, err := base64.StdEncoding.DecodeString(args.PngBase64)
	if err != nil || len(data) > maxPngBytes || !bytes.HasPrefix(data, pngMagic) {
		return ProofSavePngToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.notPng.body", nil)
	}
	path := filepath.Join(dir, safePngName(args.Filename))
	if _, err := os.Lstat(path); err == nil {
		return ProofSavePngToResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotExists.body", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProofSavePngToResult{}, ipcerr.InternalErr(err)
	}
	if err := renameio.WriteFile(path, data, 0o644); err != nil {
		return ProofSavePngToResult{}, ipcerr.InternalErr(err)
	}
	return ProofSavePngToResult{Path: path}, nil
}

// ForgetShotFolder releases a folder id once a save-all walk ends.
func (s *ProofService) ForgetShotFolder(args ProofForgetShotFolderArgs) error {
	s.shots.forget(args.FolderID)
	return nil
}
