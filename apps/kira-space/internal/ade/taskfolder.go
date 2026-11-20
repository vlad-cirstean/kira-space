package ade

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"

	"github.com/kirathecat/kira-studio/internal/fileservice"
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
	if err := fileservice.HostPrivate().MkdirAll(dir); err != nil {
		return "", fmt.Errorf("create task files folder: %w", err)
	}
	return dir, nil
}

// childFolders are the task folders of an orchestrator's existing children, which state.yaml chain
// points at. The grant is fixed at launch: a child made later is covered by chain.children[].preview and
// the next launch. Other tasks get none.
func (b *TaskBoard) childFolders(taskID string) []string {
	if b.deps.Tasks == nil || !b.isOrchestrator(taskID) {
		return nil
	}
	children, _, err := b.deps.Tasks.ChildTasks(taskID, "all", maxChildTasks)
	if err != nil {
		slog.Warn("ade: list children for the task folder grant", "scope", "ade", "task", taskID, "err", err)
		return nil
	}
	var dirs []string
	for _, c := range children {
		if dir, err := b.filesDir(c.ID); err == nil && dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
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
// absolute-path form is "//<abs>". Submit restores whatever a rule misses. An orchestrator passes its
// own folder and its children's.
func taskFolderDenies(dirs ...string) []string {
	out := make([]string, 0, 2*len(kiraOwnedFiles)*len(dirs))
	for _, dir := range dirs {
		for _, f := range taskFolderDenyFiles(dir) {
			out = append(out, "Edit(/"+filepath.ToSlash(f)+")", "Write(/"+filepath.ToSlash(f)+")")
		}
	}
	return out
}

// taskFolderAllows let a headless run write its draft and files in the task folder with no prompt;
// the denies of the Kira-owned files win over them.
func taskFolderAllows(dir string) []string {
	g := "/" + filepath.ToSlash(dir) + "/**"
	return []string{"Edit(" + g + ")", "Write(" + g + ")"}
}

// withTaskFolder adds the task folder to a headless spec.
func (b *TaskBoard) withTaskFolder(spec harness.HeadlessSpec, taskID string) (harness.HeadlessSpec, error) {
	dir, err := b.filesDir(taskID)
	if err != nil || dir == "" {
		return spec, err
	}
	if err := fileservice.HostPrivate().MkdirAll(filepath.Join(dir, "handovers")); err != nil {
		return spec, fmt.Errorf("create handovers folder: %w", err)
	}
	b.writeStateNow(taskID)
	children := b.childFolders(taskID)
	spec.AddDirs = appendMissing(slices.Clone(spec.AddDirs), append([]string{dir}, children...)...)
	spec.AllowedTools = appendMissing(slices.Clone(spec.AllowedTools), taskFolderAllows(dir)...)
	spec.DisallowedTools = appendMissing(slices.Clone(spec.DisallowedTools), taskFolderDenies(append([]string{dir}, children...)...)...)
	return spec, nil
}

// withTaskFolderArgs adds the task folder to an interactive launch.
func (b *TaskBoard) withTaskFolderArgs(pa *PrepareArgs, taskID string) error {
	dir, err := b.filesDir(taskID)
	if err != nil || dir == "" {
		return err
	}
	if err := fileservice.HostPrivate().MkdirAll(filepath.Join(dir, "handovers")); err != nil {
		return fmt.Errorf("create handovers folder: %w", err)
	}
	b.writeStateNow(taskID)
	children := b.childFolders(taskID)
	pa.AddDirs = appendMissing(slices.Clone(pa.AddDirs), append([]string{dir}, children...)...)
	pa.DisallowedTools = appendMissing(slices.Clone(pa.DisallowedTools), taskFolderDenies(append([]string{dir}, children...)...)...)
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
