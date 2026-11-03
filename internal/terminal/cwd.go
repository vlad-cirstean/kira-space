//go:build linux || darwin

package terminal

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"
)

// cwdTimeout bounds one process cwd lookup.
const cwdTimeout = 2 * time.Second

// liveCwd is the working directory of the pty's foreground process group, else the shell's, else
// the start cwd. A running tool prints paths relative to its own cwd, which `cd` changes.
func (s *Session) liveCwd() string {
	ctx, cancel := context.WithTimeout(context.Background(), cwdTimeout)
	defer cancel()
	if pgid := s.foregroundPgid(); pgid > 0 {
		if dir := processCwd(ctx, pgid); dir != "" {
			return dir
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		if dir := processCwd(ctx, int32(s.cmd.Process.Pid)); dir != "" {
			return dir
		}
	}
	return s.cwd
}

// foregroundPgid reads TIOCGPGRP through SyscallConn; ptmx.Fd() would force blocking mode.
func (s *Session) foregroundPgid() int32 {
	rc, err := s.ptmx.SyscallConn()
	if err != nil {
		return 0
	}
	var pgid int
	var ioErr error
	if err := rc.Control(func(fd uintptr) {
		pgid, ioErr = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	}); err != nil || ioErr != nil {
		return 0
	}
	return int32(pgid)
}

func processCwd(ctx context.Context, pid int32) string {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return ""
	}
	dir, err := p.CwdWithContext(ctx)
	if err != nil {
		return ""
	}
	return dir
}
