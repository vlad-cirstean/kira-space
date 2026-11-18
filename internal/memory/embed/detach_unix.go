package embed

import (
	"os/exec"
	"syscall"
)

// detach puts the shared worker in its own session: it must outlive the client that started it and
// the signals aimed at that client's process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
