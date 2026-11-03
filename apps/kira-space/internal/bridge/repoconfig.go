package bridge

import "github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"

// ChannelRepoConfigChanged is pushed, payload-free, when a repo file changes on disk or is saved, or
// the set of file errors changes.
const ChannelRepoConfigChanged = "kira:repoConfig:changed"

// RepoConfigService is Kira Space's own binding-name shim over repocfg.Service.
type RepoConfigService struct {
	*repocfg.Service
}

// RepoConfigChanged is the store's reload target.
func RepoConfigChanged(ev *Events) { ev.Broadcast(ChannelRepoConfigChanged) }
