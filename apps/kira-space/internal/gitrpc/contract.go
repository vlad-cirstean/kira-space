// Package gitrpc is the git method table behind GitService and the graph data plane: one request
// handler per git method, one graph stream handler, and the wire shapes they exchange. The wire
// shapes are mirrored by hand in packages/git-ipc/src/contract.ts and pinned by the contract
// fixtures under apps/kira-space/tests/contract.
package gitrpc
