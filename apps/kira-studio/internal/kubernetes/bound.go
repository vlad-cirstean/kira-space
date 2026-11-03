package kubernetes

import (
	"context"

	"github.com/kirathecat/kira-studio/internal/appevent"
)

// BoundService is the Wails-bound Kubernetes surface. Studio's bridge.KubernetesService embeds it
// so the binding name stays app-local.
type BoundService struct {
	s *Service
}

// NewBoundService builds the bound surface over a fresh Service.
func NewBoundService(store ClusterStore, emit appevent.Emitter, opts ...Option) *BoundService {
	return &BoundService{s: NewService(store, emit, opts...)}
}

// ServiceOf returns the domain service behind b. A package function, not a method: Wails binds
// every exported method.
func ServiceOf(b *BoundService) *Service { return b.s }

// CloseWindowBound ends every log stream windowKey started. A package function for the same reason.
func CloseWindowBound(b *BoundService, windowKey string) { b.s.CloseWindow(windowKey) }

// ServiceShutdown ends every stream at app quit. Wails calls it and leaves it out of the bindings.
func (b *BoundService) ServiceShutdown() error {
	b.s.Shutdown()
	return nil
}

// Clusters lists the kubeconfig contexts and the kubeconfig state.
func (b *BoundService) Clusters() (ClustersResult, error) { return b.s.Clusters(context.Background()) }

// Status probes one cluster; Refresh reloads its client first.
func (b *BoundService) Status(args StatusArgs) (Status, error) {
	return b.s.Status(context.Background(), args)
}

// SetMcp stores a cluster's MCP description and per-operation modes.
func (b *BoundService) SetMcp(args SetMcpArgs) (McpSettings, error) {
	return b.s.SetMcp(context.Background(), args)
}

// Reorder stores the user's cluster order and returns the reordered list.
func (b *BoundService) Reorder(args ReorderArgs) (ClustersResult, error) {
	return b.s.Reorder(context.Background(), args)
}

func (b *BoundService) Namespaces(args ClusterArgs) (NamespacesResult, error) {
	return b.s.Namespaces(context.Background(), args)
}

func (b *BoundService) Kinds(args ClusterArgs) (KindsResult, error) {
	return b.s.Kinds(context.Background(), args)
}

func (b *BoundService) List(args ListArgs) (ListResult, error) {
	return b.s.List(context.Background(), args)
}

func (b *BoundService) Describe(args DescribeArgs) (Description, error) {
	return b.s.Describe(context.Background(), args)
}

func (b *BoundService) LogsOpen(args LogsOpenArgs) error { return b.s.LogsOpen(args) }

func (b *BoundService) LogsClose(args StreamArgs) error {
	b.s.LogsClose(args.StreamID)
	return nil
}
