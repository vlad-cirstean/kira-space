package bridge

import "github.com/kirathecat/kira-studio/internal/approvals"

// ApprovalsService is this app's binding-name shim over approvals.Service.
type ApprovalsService struct {
	*approvals.Service
}
