package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// LeaseBoard is the engine side of resource leases; *ade.TaskBoard implements it.
type LeaseBoard interface {
	LeaseSnapshot() adewire.LeaseSnapshot
	SaveLeaseQueues(names []string) error
	RequestLeaseRelease(queue string) error
	ForceLeaseRelease(queue string) error
}

// LeasesService exposes the lease queues: the panel lists them, the user names them and acts on a holder.
type LeasesService struct{ Board LeaseBoard }

// AdeLeasesChanged is TaskBoard.OnLeases' target: payload-free.
func AdeLeasesChanged(ev *Events) { ev.Broadcast(adewire.ChannelLeases) }

func (s *LeasesService) Snapshot(_ context.Context) (adewire.LeaseSnapshot, error) {
	return s.Board.LeaseSnapshot(), nil
}

func (s *LeasesService) SaveQueues(_ context.Context, args adewire.LeaseQueuesArgs) error {
	return adeTaskError(s.Board.SaveLeaseQueues(args.Names))
}

func (s *LeasesService) RequestRelease(_ context.Context, args adewire.LeaseQueueArgs) error {
	return adeTaskError(s.Board.RequestLeaseRelease(args.Queue))
}

func (s *LeasesService) ForceRelease(_ context.Context, args adewire.LeaseQueueArgs) error {
	return adeTaskError(s.Board.ForceLeaseRelease(args.Queue))
}
