// Package migrations embeds kira.db's schema steps. 0090 is the v0.3.1 baseline; later steps
// number from 0091.
package migrations

import (
	"embed"

	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

//go:embed *.sql
var files embed.FS

// names lists the embedded files in the exact order they must apply, rather than trusting
// directory listing order.
var names = []sqlitex.MigrationSource{
	{Version: 90, Name: "p557_baseline", File: "0090_p557_baseline.sql"},
	{Version: 91, Name: "p557_drop_legacy", File: "0091_p557_drop_legacy.sql"},
	{Version: 92, Name: "p569_proof_image_view", File: "0092_p569_proof_image_view.sql"},
	{Version: 93, Name: "p571_orchestrator", File: "0093_p571_orchestrator.sql"},
	{Version: 94, Name: "p583_adopted", File: "0094_p583_adopted.sql"},
	{Version: 95, Name: "p593_explain_threshold", File: "0095_p593_explain_threshold.sql"},
	{Version: 96, Name: "p595_api_default", File: "0096_p595_api_default.sql"},
	{Version: 97, Name: "p603_kube_mcp_access", File: "0097_p603_kube_mcp_access.sql"},
}

// All returns every migration in ascending version order.
func All() ([]sqlitex.Migration, error) {
	return sqlitex.LoadMigrations(files, names)
}
