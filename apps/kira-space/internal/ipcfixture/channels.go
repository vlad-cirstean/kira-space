// Package ipcfixture is the Go successor to tests/ipc/'s TypeScript fixture generator.
// It drives the real app stack — storage, repos, connections.Service, tree.Service, the bridge
// services, and a real adapterhost.Router/Dispatcher — against a real adapter container, and
// either asserts the result against a committed fixture (the default) or writes a fresh one
// (KIRA_IPC_FIXTURES=write). Six per-adapter *_test.go files share the machinery in this
// file plus decode.go, write.go and frozen.go.
package ipcfixture

// The six IPC channels every committed fixture needs (P58f §4.3(a)) — grepped verbatim from
// packages/shared/protocol/ipc.ts's IPC map, never inferred from the TypeScript identifier (CLAUDE.md's
// P52-P56 finding: ENGINE_OP.configureCache is 'cache:configure', not 'engine:configure-cache').
const (
	channelConnectionsList    = "kira:connections:list"
	channelConnectionsStates  = "kira:connections:states"
	channelConnectionsConnect = "kira:connections:connect"
	channelTreeChildren       = "kira:tree:children"
	channelTreeDescribe       = "kira:tree:describe"
	channelTreeDefinition     = "kira:tree:definition"
	channelOpsRecent          = "kira:ops:recent"
	channelOpsCancel          = "kira:ops:cancel"
)

// The two DATA_OP values every committed fixture needs, from packages/shared/protocol/dataOps.ts's
// DATA_OP map — same discipline as the channel constants above.
const (
	dataOpRead       = "data:read"
	dataOpCount      = "data:count"
	dataOpExecute    = "data:execute"
	dataOpMutate     = "data:mutate"
	dataOpInvalidate = "data:invalidate"
)
