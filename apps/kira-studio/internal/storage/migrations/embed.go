// Package migrations embeds the schema migration(s). The app has never shipped, so there is no
// installed base with a partially-applied schema to preserve — what were five incremental steps
// are collapsed into the single 0001_init.sql that produces the exact same final schema in one
// shot.
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
	{Version: 1, Name: "init", File: "0001_init.sql"},
	{Version: 2, Name: "p8_windows", File: "0002_p8_windows.sql"},
	{Version: 3, Name: "p18_connection_ddl", File: "0003_p18_connection_ddl.sql"},
	{Version: 4, Name: "p18_auto_explain", File: "0004_p18_auto_explain.sql"},
	{Version: 5, Name: "p28_throttle", File: "0005_p28_throttle.sql"},
	{Version: 6, Name: "p4_collections", File: "0006_p4_collections.sql"},
	{Version: 7, Name: "p5_variables", File: "0007_p5_variables.sql"},
	{Version: 8, Name: "p8_response_history", File: "0008_p8_response_history.sql"},
	{Version: 9, Name: "p11_grpc", File: "0009_p11_grpc.sql"},
	{Version: 10, Name: "p12_api_rename", File: "0010_p12_api_rename.sql"},
	{Version: 11, Name: "p17_variable_description", File: "0011_p17_variable_description.sql"},
	{Version: 12, Name: "p18_environment_color", File: "0012_p18_environment_color.sql"},
	{Version: 13, Name: "p21r3_history_bytes_index", File: "0013_p21r3_history_bytes_index.sql"},
	{Version: 14, Name: "p22_window_mode", File: "0014_p22_window_mode.sql"},
	{Version: 15, Name: "p23_op_log_bytes", File: "0015_p23_op_log_bytes.sql"},
	{Version: 16, Name: "g1_git_clients", File: "0016_g1_git_clients.sql"},
	{Version: 17, Name: "g18_git_repo_settings", File: "0017_g18_git_repo_settings.sql"},
	{Version: 18, Name: "c5_code_repos", File: "0018_c5_code_repos.sql"},
	{Version: 19, Name: "p67d_repo_map_access", File: "0019_p67d_repo_map_access.sql"},
	{Version: 20, Name: "m1_connection_mcp", File: "0020_m1_connection_mcp.sql"},
	{Version: 21, Name: "m2_connection_permissions", File: "0021_m2_connection_permissions.sql"},
	{Version: 22, Name: "m3_connection_mcp_explain", File: "0022_m3_connection_mcp_explain.sql"},
	{Version: 23, Name: "m5_column_mask_rules", File: "0023_m5_column_mask_rules.sql"},
	{Version: 24, Name: "p85_custom_scripts", File: "0024_p85_custom_scripts.sql"},
	{Version: 25, Name: "p97_drop_repo_map", File: "0025_p97_drop_repo_map.sql"},
	{Version: 26, Name: "p100_drop_git_tables", File: "0026_p100_drop_git_tables.sql"},
	{Version: 27, Name: "p108part11_op_log_path", File: "0027_p108part11_op_log_path.sql"},
	{Version: 28, Name: "p120_drop_git_settings", File: "0028_p120_drop_git_settings.sql"},
	{Version: 29, Name: "p127_drop_agent_hooks_settings", File: "0029_p127_drop_agent_hooks_settings.sql"},
	{Version: 30, Name: "p168_fk_indexes", File: "0030_p168_fk_indexes.sql"},
	{Version: 31, Name: "p187_custom_script_collection", File: "0031_p187_custom_script_collection.sql"},
	{Version: 32, Name: "p188_drop_claude_code_settings", File: "0032_p188_drop_claude_code_settings.sql"},
	{Version: 33, Name: "p219_quick_command_collections", File: "0033_p219_quick_command_collections.sql"},
	{Version: 34, Name: "p242_automations", File: "0034_p242_automations.sql"},
	{Version: 35, Name: "p242_smart_scripts", File: "0035_p242_smart_scripts.sql"},
	{Version: 36, Name: "p242_scripts_in_ade", File: "0036_p242_scripts_in_ade.sql"},
	{Version: 37, Name: "p242_recurring_scripts", File: "0037_p242_recurring_scripts.sql"},
	{Version: 38, Name: "p279_script_run_resume", File: "0038_p279_script_run_resume.sql"},
	{Version: 39, Name: "p312_layout_per_mode", File: "0039_p312_layout_per_mode.sql"},
	{Version: 40, Name: "p307_preconnect_delay", File: "0040_p307_preconnect_delay.sql"},
	{Version: 41, Name: "p316_script_run_provider", File: "0041_p316_script_run_provider.sql"},
	{Version: 42, Name: "p316_agent_launch_kind", File: "0042_p316_agent_launch_kind.sql"},
	{Version: 43, Name: "p328_op_log_source", File: "0043_p328_op_log_source.sql"},
	{Version: 44, Name: "p334_docker_restore", File: "0044_p334_docker_restore.sql"},
	{Version: 45, Name: "p382_connection_mcp_review", File: "0045_p382_connection_mcp_review.sql"},
	{Version: 46, Name: "p420_validation_proof", File: "0046_p420_validation_proof.sql"},
	{Version: 47, Name: "p425_discover_saved_queries", File: "0047_p425_discover_saved_queries.sql"},
	{Version: 48, Name: "p426_proof_chart_view", File: "0048_p426_proof_chart_view.sql"},
	{Version: 49, Name: "p439_kube_clusters", File: "0049_p439_kube_clusters.sql"},
	{Version: 50, Name: "p441_script_run_restore", File: "0050_p441_script_run_restore.sql"},
	{Version: 51, Name: "p440_kube_cluster_mcp", File: "0051_p440_kube_cluster_mcp.sql"},
	{Version: 52, Name: "p447_secret_values", File: "0052_p447_secret_values.sql"},
}

// All returns every migration in ascending version order.
func All() ([]sqlitex.Migration, error) {
	return sqlitex.LoadMigrations(files, names)
}
