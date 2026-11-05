package adapters

import (
	"maps"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// LaneOption marks a config opened as a DB MCP read lane (Options key): drivers that pool on
// their own cap the pool, since a lane serves one agent call at a time.
const LaneOption = "kiraReadLane"

// LanePoolSize is the pool cap for a lane on a driver that pools on its own.
const LanePoolSize = 2

// LaneConfig is cfg for a read lane: the read-only switch on and the pool cap marker, same
// credentials.
func LaneConfig(cfg model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
	lane := cfg
	lane.ReadOnly = true
	lane.Options = maps.Clone(cfg.Options)
	if lane.Options == nil {
		lane.Options = make(map[string]any, 1)
	}
	lane.Options[LaneOption] = true
	return lane
}

// IsLaneConfig reports whether options open a read lane.
func IsLaneConfig(options map[string]any) bool {
	lane, _ := options[LaneOption].(bool)
	return lane
}
