package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/internal/agentport"
)

// AgentPortService is Kira Space's binding of agentport: Import and Export of a provider's native
// user-level agent config. It lists methods one by one so the service's Close stays unbound.
type AgentPortService struct {
	Svc *agentport.Service
}

// List names the providers with an Import and an Export, and where their native files are.
func (s *AgentPortService) List(ctx context.Context) []agentport.PortProvider {
	return s.Svc.List(ctx)
}

// ImportPreview reads the provider's native files and lists what an import would bring in.
func (s *AgentPortService) ImportPreview(ctx context.Context, args agentport.ImportPreviewArgs) (agentport.ImportPreview, error) {
	return s.Svc.ImportPreview(ctx, args)
}

// ImportApply writes the selected items of a preview.
func (s *AgentPortService) ImportApply(ctx context.Context, args agentport.ImportApplyArgs) (agentport.PortResult, error) {
	return s.Svc.ImportApply(ctx, args)
}

// ExportPreview renders the app's settings and context as the provider's native files, with a diff.
func (s *AgentPortService) ExportPreview(ctx context.Context, args agentport.ExportPreviewArgs) (agentport.ExportPreview, error) {
	return s.Svc.ExportPreview(ctx, args)
}

// ExportApply writes the confirmed files after a backup and registers the MCP servers.
func (s *AgentPortService) ExportApply(ctx context.Context, args agentport.ExportApplyArgs) (agentport.PortResult, error) {
	return s.Svc.ExportApply(ctx, args)
}

// Cancel drops a preview.
func (s *AgentPortService) Cancel(args agentport.CancelArgs) { s.Svc.Cancel(args) }
