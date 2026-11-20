// Package editorlaunch finds VS Code and opens paths in it as a detached process. It runs no shell:
// argv only, never a .cmd on Windows, and it reads no credential.
package editorlaunch

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotFound marks a machine with no VS Code install this package can find.
var ErrNotFound = errors.New("editorlaunch: VS Code not found")

// Locate returns the VS Code executable for goos. PATH comes first, then the usual install paths.
// getenv reads the Windows install roots; home is the user's home directory.
func Locate(goos, home string, getenv func(string) string) (string, error) {
	return locate(goos, home, getenv, exec.LookPath, isFile)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func locate(goos, home string, getenv func(string) string, lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	if goos == "windows" {
		return locateWindows(getenv, lookPath, exists)
	}
	if p, err := lookPath("code"); err == nil {
		return p, nil
	}
	var candidates []string
	if goos == "darwin" {
		const app = "Visual Studio Code.app/Contents/Resources/app/bin/code"
		candidates = []string{
			"/Applications/" + app,
			filepath.Join(home, "Applications", app),
			"/opt/homebrew/bin/code",
			"/usr/local/bin/code",
		}
	} else {
		candidates = []string{"/usr/bin/code", "/snap/bin/code", "/usr/share/code/bin/code"}
	}
	for _, c := range candidates {
		if exists(c) {
			return c, nil
		}
	}
	return "", ErrNotFound
}

// locateWindows never returns a .cmd: a PATH hit on code.cmd maps to Code.exe beside its bin dir.
func locateWindows(getenv func(string) string, lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	if p, err := lookPath("code.cmd"); err == nil {
		exe := filepath.Join(filepath.Dir(filepath.Dir(p)), "Code.exe")
		if exists(exe) {
			return exe, nil
		}
	}
	var candidates []string
	if v := getenv("LOCALAPPDATA"); v != "" {
		candidates = append(candidates, filepath.Join(v, "Programs", "Microsoft VS Code", "Code.exe"))
	}
	if v := getenv("ProgramFiles"); v != "" {
		candidates = append(candidates, filepath.Join(v, "Microsoft VS Code", "Code.exe"))
	}
	for _, c := range candidates {
		if exists(c) && !strings.HasSuffix(strings.ToLower(c), ".cmd") {
			return c, nil
		}
	}
	return "", ErrNotFound
}
