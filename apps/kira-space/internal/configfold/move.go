package configfold

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/kirathecat/kira-studio/internal/fsmove"
)

// backup copies the legacy entry into the backup folder; a legacy symlink is kept as a link.
func backup(e entry, home, dir string) error {
	src, dst := e.src(home), filepath.Join(dir, e.name)
	if !fsmove.Exists(src) || fsmove.Exists(dst) {
		return nil
	}
	return copyVerified(src, dst)
}

// copyVerified copies src to dst through dst+".tmp" and a verify, so dst is never half written.
func copyVerified(src, dst string) error {
	tmp := dst + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("configfold: %w", err)
	}
	if err := fsmove.CopyTree(src, tmp); err != nil {
		return fmt.Errorf("configfold: copy %s: %w", src, err)
	}
	if err := fsmove.VerifyTree(src, tmp); err != nil {
		return fmt.Errorf("configfold: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("configfold: %w", err)
	}
	return nil
}

// move lands one legacy entry in its target. Every outcome is safe to repeat.
func (o *Options) move(e entry) error {
	src, dst := e.src(o.Home), e.dst(o.Home)
	srcInfo, err := os.Lstat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("configfold: %w", err)
	}
	dstInfo, err := os.Lstat(dst)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return o.relocate(src, dst)
	case err != nil:
		return fmt.Errorf("configfold: %w", err)
	}
	if srcInfo.IsDir() && dstInfo.IsDir() {
		return mergeDir(src, dst)
	}
	slog.Warn("config fold: target exists, legacy entry stays only in the backup",
		"scope", "startup", "legacy", src, "target", dst)
	return removeLegacy(src)
}

func removeLegacy(src string) error {
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("configfold: %w", err)
	}
	return nil
}

// relocate renames src to the missing dst; across devices it copies, verifies, then removes src.
func (o *Options) relocate(src, dst string) error {
	err := o.rename(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return fmt.Errorf("configfold: move %s: %w", src, err)
	}
	if err := copyVerified(src, dst); err != nil {
		return err
	}
	return removeLegacy(src)
}

// mergeDir keeps dst: a child of src that dst lacks moves in, a differing child stays only in the backup.
func mergeDir(src, dst string) error {
	children, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("configfold: %w", err)
	}
	for _, c := range children {
		from, to := filepath.Join(src, c.Name()), filepath.Join(dst, c.Name())
		if !fsmove.Exists(to) {
			if err := os.Rename(from, to); err != nil {
				return fmt.Errorf("configfold: merge %s: %w", from, err)
			}
			continue
		}
		if fsmove.VerifyTree(from, to) != nil {
			slog.Warn("config fold: target exists, legacy entry stays only in the backup",
				"scope", "startup", "legacy", from, "target", to)
		}
	}
	return removeLegacy(src)
}
