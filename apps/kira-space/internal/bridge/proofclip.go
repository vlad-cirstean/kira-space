package bridge

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// ProofShotClipboard says whether the system clipboard can hold several images.
type ProofShotClipboard struct {
	MultiImage bool `json:"multiImage"`
}

// ProofShotClipResult is a fresh clipboard batch.
type ProofShotClipResult struct {
	ID string `json:"id"`
}

// ProofAddShotClipArgs adds one PNG to a batch from OpenShotClip.
type ProofAddShotClipArgs struct {
	ClipID    string `json:"clipId"`
	Filename  string `json:"filename"`
	PngBase64 string `json:"pngBase64"`
}

// ProofShotClipArgs names a batch from OpenShotClip.
type ProofShotClipArgs struct {
	ClipID string `json:"clipId"`
}

// ProofCommitShotClipResult is the number of images now on the clipboard.
type ProofCommitShotClipResult struct {
	Count int `json:"count"`
}

func (s *ProofService) multiImage() bool {
	return s.Clipboard != nil && s.Clipboard.MultiImage()
}

func clipboardUnavailableErr() error {
	return ipcerr.T(ipcerr.CodeUnavailable, "backend.studio.proof.clipboardUnavailable.body", nil)
}

func clipGoneErr() error {
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotFolderGone.body", nil)
}

// ShotClipboard reports whether copy-all is possible on this system.
func (s *ProofService) ShotClipboard() (ProofShotClipboard, error) {
	return ProofShotClipboard{MultiImage: s.multiImage()}, nil
}

// OpenShotClip starts a clipboard batch in a fresh temp directory.
func (s *ProofService) OpenShotClip() (ProofShotClipResult, error) {
	if !s.multiImage() {
		return ProofShotClipResult{}, clipboardUnavailableErr()
	}
	dir, err := os.MkdirTemp("", "kira-proof-clip-")
	if err != nil {
		return ProofShotClipResult{}, ipcerr.InternalErr(err)
	}
	id, evicted := s.clips.add(dir)
	if evicted != nil && !evicted.closed {
		// An abandoned batch (its window closed mid-walk) would keep its files in TMPDIR.
		removeClipDir(evicted.path)
	}
	return ProofShotClipResult{ID: id}, nil
}

func removeClipDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Debug("proof: remove clip batch", "scope", "proof", "path", path, "err", err)
	}
}

// ServiceShutdown removes every batch's files at app quit: they are screenshots and must not outlive it.
// Wails calls it and leaves it out of the bindings.
func (s *ProofService) ServiceShutdown() error {
	for _, path := range s.clips.unclosed() {
		removeClipDir(path)
	}
	s.clipMu.Lock()
	defer s.clipMu.Unlock()
	if s.lastClip != "" {
		removeClipDir(s.lastClip)
		s.lastClip = ""
	}
	return nil
}

// AddShotClip writes one PNG into an open batch, never over an existing file.
func (s *ProofService) AddShotClip(args ProofAddShotClipArgs) error {
	clip, ok := s.clips.get(args.ClipID)
	if !ok || clip.closed {
		return clipGoneErr()
	}
	data, err := decodeShot(args.PngBase64, pngMagic, "backend.studio.proof.notPng.body")
	if err != nil {
		return err
	}
	path := filepath.Join(clip.path, safePngName(args.Filename))
	if _, err := os.Lstat(path); err == nil {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.shotExists.body", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ipcerr.InternalErr(err)
	}
	if err := renameio.WriteFile(path, data, 0o644); err != nil {
		return ipcerr.InternalErr(err)
	}
	return nil
}

// CommitShotClip puts a batch's PNGs on the clipboard, one item each in file-name order. The previous
// batch's files go only after the new write succeeds.
func (s *ProofService) CommitShotClip(args ProofShotClipArgs) (ProofCommitShotClipResult, error) {
	clip, ok := s.clips.get(args.ClipID)
	if !ok || clip.closed {
		return ProofCommitShotClipResult{}, clipGoneErr()
	}
	if !s.multiImage() {
		return ProofCommitShotClipResult{}, clipboardUnavailableErr()
	}
	entries, err := os.ReadDir(clip.path)
	if err != nil {
		return ProofCommitShotClipResult{}, ipcerr.InternalErr(err)
	}
	var paths []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			paths = append(paths, filepath.Join(clip.path, e.Name()))
		}
	}
	if len(paths) == 0 {
		return ProofCommitShotClipResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.clipEmpty.body", nil)
	}
	// One lock spans the write and the swap: two commits cannot each remove the other's live batch.
	s.clipMu.Lock()
	defer s.clipMu.Unlock()
	if err := s.Clipboard.WriteImages(paths); err != nil {
		return ProofCommitShotClipResult{}, ipcerr.InternalErr(err)
	}
	s.clips.close(args.ClipID)
	if s.lastClip != "" && s.lastClip != clip.path {
		removeClipDir(s.lastClip)
	}
	s.lastClip = clip.path
	return ProofCommitShotClipResult{Count: len(paths)}, nil
}

// CloseShotClip drops an uncommitted batch and its files. A committed or unknown id is a no-op: the
// clipboard items point at a committed batch's files.
func (s *ProofService) CloseShotClip(args ProofShotClipArgs) error {
	clip, ok := s.clips.get(args.ClipID)
	if !ok || clip.closed {
		return nil
	}
	s.clips.close(args.ClipID)
	removeClipDir(clip.path)
	return nil
}
