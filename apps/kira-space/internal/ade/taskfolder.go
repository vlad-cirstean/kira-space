package ade

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// taskfolder.go grants an agent its task's file folder (P643): --add-dir, a sandbox write allow,
// and deny rules on the Kira-owned files. Nothing else under the tasks root is granted.

// kiraOwnedFiles are the task folder files only Kira writes; edits to them are undone at submit.
var kiraOwnedFiles = []string{"README.md", "task.yaml", "state.yaml"}

// filesDir is the task's file folder, created. "" means the board has no files root.
func (b *TaskBoard) filesDir(taskID string) (string, error) {
	if b.deps.FilesDir == "" {
		return "", nil
	}
	dir := filepath.Join(b.deps.FilesDir, pathSegment(taskID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create task files folder: %w", err)
	}
	return dir, nil
}

// taskFolderDenyFiles are the absolute paths of the Kira-owned files of dir.
func taskFolderDenyFiles(dir string) []string {
	out := make([]string, len(kiraOwnedFiles))
	for i, f := range kiraOwnedFiles {
		out[i] = filepath.Join(dir, f)
	}
	return out
}

// taskFolderDenies are the deny rules that back the sandbox on a host with no Seatbelt: the Claude
// absolute-path form is "//<abs>". Submit restores whatever a rule misses.
func taskFolderDenies(dir string) []string {
	out := make([]string, 0, 2*len(kiraOwnedFiles))
	for _, f := range taskFolderDenyFiles(dir) {
		out = append(out, "Edit(/"+filepath.ToSlash(f)+")", "Write(/"+filepath.ToSlash(f)+")")
	}
	return out
}

// withTaskFolder adds the task folder to a headless spec.
func (b *TaskBoard) withTaskFolder(spec harness.HeadlessSpec, taskID string) (harness.HeadlessSpec, error) {
	dir, err := b.filesDir(taskID)
	if err != nil || dir == "" {
		return spec, err
	}
	spec.AddDirs = appendMissing(slices.Clone(spec.AddDirs), dir)
	spec.DisallowedTools = appendMissing(slices.Clone(spec.DisallowedTools), taskFolderDenies(dir)...)
	return spec, nil
}

// withTaskFolderArgs adds the task folder to an interactive launch.
func (b *TaskBoard) withTaskFolderArgs(pa *PrepareArgs, taskID string) error {
	dir, err := b.filesDir(taskID)
	if err != nil || dir == "" {
		return err
	}
	pa.AddDirs = appendMissing(slices.Clone(pa.AddDirs), dir)
	pa.DisallowedTools = appendMissing(slices.Clone(pa.DisallowedTools), taskFolderDenies(dir)...)
	return nil
}

func appendMissing(dst []string, add ...string) []string {
	for _, s := range add {
		if !slices.Contains(dst, s) {
			dst = append(dst, s)
		}
	}
	return dst
}
