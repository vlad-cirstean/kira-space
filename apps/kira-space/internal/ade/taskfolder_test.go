package ade

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/internal/harness"
)

func TestWithTaskFolder(t *testing.T) {
	root := t.TempDir()
	b := &TaskBoard{deps: TaskBoardDeps{FilesDir: root}}
	dir := filepath.Join(root, "t1")
	spec, err := b.withTaskFolder(harness.HeadlessSpec{AddDirs: []string{"/w"}, DisallowedTools: []string{"Bash"}}, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if len(spec.AddDirs) != 2 || spec.AddDirs[1] != dir {
		t.Fatalf("add dirs %v", spec.AddDirs)
	}
	want := []string{"Bash"}
	for _, f := range []string{"README.md", "task.yaml", "state.yaml"} {
		want = append(want, "Edit(/"+dir+"/"+f+")", "Write(/"+dir+"/"+f+")")
	}
	if len(spec.DisallowedTools) != len(want) {
		t.Fatalf("denies %v", spec.DisallowedTools)
	}
	for i := range want {
		if spec.DisallowedTools[i] != want[i] {
			t.Fatalf("deny %d = %q, want %q", i, spec.DisallowedTools[i], want[i])
		}
	}
	pa := PrepareArgs{AddDirs: []string{dir}}
	if err := b.withTaskFolderArgs(&pa, "t1"); err != nil || len(pa.AddDirs) != 1 || len(pa.DisallowedTools) != 6 {
		t.Fatalf("args %+v %v", pa, err)
	}
	none := &TaskBoard{}
	if s, err := none.withTaskFolder(harness.HeadlessSpec{}, "t1"); err != nil || len(s.AddDirs) != 0 {
		t.Fatalf("no root granted %v %v", s.AddDirs, err)
	}
}
