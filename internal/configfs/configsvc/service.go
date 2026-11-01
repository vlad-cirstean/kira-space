// Package configsvc is the one Wails-bound config-files surface both apps embed: open a config file
// in the OS default app or reveal it in the file manager. The renderer names a registered folder and a
// file in it; no path from the renderer reaches the OS.
package configsvc

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// FileOpener is the OS seam: the default-app open and the file-manager reveal.
type FileOpener interface {
	OpenFile(path string) error
	RevealFile(path string) error
}

// Service is the shared bound type; each app's own bridge type embeds it so binding names stay app-local.
type Service struct {
	opener  FileOpener
	folders map[string]string
}

// New registers folders as name to directory.
func New(opener FileOpener, folders map[string]string) *Service {
	return &Service{opener: opener, folders: folders}
}

// FileArgs names one config file: a registered folder and a file name in it.
type FileArgs struct {
	Folder string `json:"folder"`
	File   string `json:"file"`
}

// Open opens the file in the OS default app.
func (s *Service) Open(args FileArgs) error {
	p, err := s.resolve(args)
	if err != nil {
		return err
	}
	if err := s.opener.OpenFile(p); err != nil {
		return ipcerr.InternalErr(err)
	}
	return nil
}

// Reveal shows the file in the file manager.
func (s *Service) Reveal(args FileArgs) error {
	p, err := s.resolve(args)
	if err != nil {
		return err
	}
	if err := s.opener.RevealFile(p); err != nil {
		return ipcerr.InternalErr(err)
	}
	return nil
}

func (s *Service) resolve(args FileArgs) (string, error) {
	dir, ok := s.folders[args.Folder]
	if !ok {
		return "", ipcerr.Contract("unknown config folder")
	}
	f := args.File
	if f == "" || f != filepath.Base(f) || strings.HasPrefix(f, ".") || strings.ContainsAny(f, `/\`) {
		return "", ipcerr.Contract("invalid config file name")
	}
	p := filepath.Join(dir, f)
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", ipcerr.Contract("config file does not exist")
	}
	return p, nil
}
