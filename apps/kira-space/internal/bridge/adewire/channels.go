package adewire

// Push channels of AdeTaskService. Payload-free channels invalidate the matching queries.
const (
	ChannelBoard       = "kira:adetask:board"
	ChannelWorkflows   = "kira:adetask:workflows"
	ChannelRepos       = "kira:adetask:repos"
	ChannelRuns        = "kira:adetask:runs" // RunsChangedEvent
	ChannelLog         = "kira:adetask:log"  // LogEvent
	ChannelSessions    = "kira:adetask:sessions"
	ChannelOpenSession = "kira:adetask:openSession" // OpenSessionEvent, EmitTo
	ChannelReview      = "kira:adetask:review"      // ReviewEvent
	ChannelUpdates     = "kira:adetask:updates"     // payload-free: tracked item updates changed
)
