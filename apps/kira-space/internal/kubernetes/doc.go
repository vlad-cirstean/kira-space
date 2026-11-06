// Package kubernetes is Kira Studio's read-only Kubernetes module backend (P439).
//
// It only ever issues GET requests (discovery, list, get, events, pod logs). There is no create,
// update, delete, exec, attach or port-forward path, and none may be added here.
//
// Service is the domain surface other in-app consumers (the P440 MCP tools) call; BoundService is
// the Wails surface over it.
package kubernetes
