package preconnect

import (
	"fmt"
	"syscall"
)

// signalName maps a syscall.Signal to Node's own NodeJS.Signals name. Go's Signal.String()
// returns human prose ("terminated"), which would silently change the user-visible exit message
// preconnect.ts's own wording produces (§4.4).
func signalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	case syscall.SIGABRT:
		return "SIGABRT"
	case syscall.SIGSEGV:
		return "SIGSEGV"
	case syscall.SIGPIPE:
		return "SIGPIPE"
	default:
		return fmt.Sprintf("signal %d", sig)
	}
}
