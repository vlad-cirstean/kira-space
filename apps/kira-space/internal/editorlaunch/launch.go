package editorlaunch

import (
	"os"
	"os/exec"
	"runtime"
)

// VSCode opens paths in the user's VS Code.
type VSCode struct{}

// Open starts VS Code detached with paths as argv. It never waits for it and never kills it with
// the caller. Locate runs on each call: the user may install VS Code while the app runs.
func (VSCode) Open(paths []string) error {
	home, _ := os.UserHomeDir()
	bin, err := Locate(runtime.GOOS, home, os.Getenv)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, paths...)
	cmd.Env = childEnv()
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
