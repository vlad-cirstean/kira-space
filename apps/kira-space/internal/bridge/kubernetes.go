package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
)

// KubernetesService is Kira Studio's own binding-name shim over kubernetes.BoundService, same
// reason as DockerService.
type KubernetesService struct {
	*kubernetes.BoundService
}

// KubeRecorder records each Kubernetes cluster call as a connectionless op row.
type KubeRecorder struct {
	Host *adapterhost.Host
}

// Record implements kubernetes.Recorder.
func (r KubeRecorder) Record(ctx context.Context, command string, fn func(context.Context, *kubernetes.OpNote) error) error {
	_, _, err := r.Host.RunOp(ctx, adapterhost.OpSpec{Kind: "kubernetes"}, func(ctx context.Context, op *adapters.OpCtx) (any, error) {
		op.SetCommand(command)
		note := &kubernetes.OpNote{}
		err := fn(ctx, note)
		if c := note.Command(); c != "" {
			op.SetCommand(c)
		}
		if n := note.Rows(); n != nil {
			op.SetRows(*n)
		}
		return nil, err
	})
	return err
}
