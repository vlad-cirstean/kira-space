package bridge

import "github.com/kirathecat/kira-studio/internal/docker"

// DockerService is Kira Studio's own binding-name shim over internal/docker.BoundService, same
// reason as TerminalService.
type DockerService struct {
	*docker.BoundService
}
