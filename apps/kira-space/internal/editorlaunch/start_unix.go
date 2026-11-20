//go:build !windows

package editorlaunch

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/kirathecat/kira-studio/internal/loginshell"
)

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// childEnv drops the git variables that would leak the app's own repo context into the editor.
func childEnv() []string { return loginshell.ScrubGitEnv(os.Environ()) }
