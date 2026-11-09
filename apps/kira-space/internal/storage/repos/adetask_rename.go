package repos

import (
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// RenameWorkflowID points every task (archived included) of workflow oldID at newID. Snapshots keep their
// own copy of the workflow and stay as they are.
func (r *AdeTaskRepo) RenameWorkflowID(oldID, newID string) error {
	tk := kiraschema.AdeTasks
	ub := r.Dialect.Update(tk.Table)
	ub.Set(ub.Assign(tk.WorkflowID, newID)).Where(ub.Equal(tk.WorkflowID, oldID))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: rename ade task workflow id %s: %w", oldID, err)
	}
	return nil
}
