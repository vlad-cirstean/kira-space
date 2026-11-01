// Package migrations embeds Kira Space's own schema migrations — the app shipped no installed base,
// so 0001/0002 stayed one collapsed init pair the same shape Kira Studio's own
// migrations package used (see that package's own doc comment); every later migration (0003)
// applies in the ordinary way against whatever schema is already on disk.
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
	{Version: 2, Name: "p100_tabs_layout", File: "0002_p100_tabs_layout.sql"},
	{Version: 3, Name: "p128_window_mode", File: "0003_p128_window_mode.sql"},
	{Version: 4, Name: "p129_ade_sessions", File: "0004_p129_ade_sessions.sql"},
	{Version: 5, Name: "p129_ade_queue", File: "0005_p129_ade_queue.sql"},
	{Version: 6, Name: "p135_ade_dependencies", File: "0006_p135_ade_dependencies.sql"},
	{Version: 7, Name: "p136_ade_work_type", File: "0007_p136_ade_work_type.sql"},
	{Version: 8, Name: "p144_ade_tasks", File: "0008_p144_ade_tasks.sql"},
	{Version: 9, Name: "p145_ade_marks", File: "0009_p145_ade_marks.sql"},
	{Version: 10, Name: "p146_ade_runs", File: "0010_p146_ade_runs.sql"},
	{Version: 11, Name: "p148_drop_ade_v1", File: "0011_p148_drop_ade_v1.sql"},
	{Version: 12, Name: "p150_review", File: "0012_p150_review.sql"},
	{Version: 13, Name: "p155_drop_all_agents_filter", File: "0013_p155_drop_all_agents_filter.sql"},
	{Version: 14, Name: "p156_run_launch_spec", File: "0014_p156_run_launch_spec.sql"},
	{Version: 15, Name: "p158_drop_run_todo", File: "0015_p158_drop_run_todo.sql"},
	{Version: 16, Name: "p177_ade_logs_purge", File: "0016_p177_ade_logs_purge.sql"},
	{Version: 17, Name: "p196_ade_task_workflow_snapshot", File: "0017_p196_ade_task_workflow_snapshot.sql"},
	{Version: 18, Name: "p199_ade_branch_origin", File: "0018_p199_ade_branch_origin.sql"},
	{Version: 19, Name: "p202_ade_setup_note", File: "0019_p202_ade_setup_note.sql"},
	{Version: 20, Name: "p204_custom_scripts", File: "0020_p204_custom_scripts.sql"},
	{Version: 21, Name: "p212_mobile_devices", File: "0021_p212_mobile_devices.sql"},
	{Version: 22, Name: "p212_mobile_permissions", File: "0022_p212_mobile_permissions.sql"},
	{Version: 23, Name: "p219_quick_command_collections", File: "0023_p219_quick_command_collections.sql"},
	{Version: 24, Name: "p222_code_repo_color", File: "0024_p222_code_repo_color.sql"},
	{Version: 25, Name: "p223_mobile_lan", File: "0025_p223_mobile_lan.sql"},
	{Version: 26, Name: "p242_automations", File: "0026_p242_automations.sql"},
	{Version: 27, Name: "p241_rebase", File: "0027_p241_rebase.sql"},
	{Version: 28, Name: "p242_smart_scripts", File: "0028_p242_smart_scripts.sql"},
	{Version: 29, Name: "p242_scripts_in_ade", File: "0029_p242_scripts_in_ade.sql"},
	{Version: 30, Name: "p242_recurring_scripts", File: "0030_p242_recurring_scripts.sql"},
	{Version: 31, Name: "p243_drop_git_clients", File: "0031_p243_drop_git_clients.sql"},
	{Version: 32, Name: "p259_repo_hidden", File: "0032_p259_repo_hidden.sql"},
	{Version: 33, Name: "p266_ade_panel_width", File: "0033_p266_ade_panel_width.sql"},
	{Version: 34, Name: "p277_repo_hidden_rev", File: "0034_p277_repo_hidden_rev.sql"},
	{Version: 35, Name: "p279_script_run_resume", File: "0035_p279_script_run_resume.sql"},
	{Version: 36, Name: "p286_review_step", File: "0036_p286_review_step.sql"},
	{Version: 37, Name: "p287_run_sandbox", File: "0037_p287_run_sandbox.sql"},
	{Version: 38, Name: "p293_flat_steps", File: "0038_p293_flat_steps.sql"},
	{Version: 39, Name: "p312_layout_per_mode", File: "0039_p312_layout_per_mode.sql"},
	{Version: 40, Name: "p316_agent_sessions", File: "0040_p316_agent_sessions.sql"},
	{Version: 41, Name: "p316_agent_settings", File: "0041_p316_agent_settings.sql"},
	{Version: 42, Name: "p316_agent_launch_kind", File: "0042_p316_agent_launch_kind.sql"},
	{Version: 43, Name: "p318_git_host_setting", File: "0043_p318_git_host_setting.sql"},
	{Version: 44, Name: "p318_git_host_names", File: "0044_p318_git_host_names.sql"},
	{Version: 45, Name: "p332_file_handler_previous", File: "0045_p332_file_handler_previous.sql"},
	{Version: 46, Name: "p341_drop_sandbox_keychain", File: "0046_p341_drop_sandbox_keychain.sql"},
	{Version: 47, Name: "p352_git_host_change_requests", File: "0047_p352_git_host_change_requests.sql"},
	{Version: 48, Name: "p352_git_host_sync_activity", File: "0048_p352_git_host_sync_activity.sql"},
	{Version: 49, Name: "p352_git_host_delta_floor", File: "0049_p352_git_host_delta_floor.sql"},
	{Version: 50, Name: "p369_drop_viewed_sync", File: "0050_p369_drop_viewed_sync.sql"},
	{Version: 51, Name: "p379_ade_task_statuses", File: "0051_p379_ade_task_statuses.sql"},
	{Version: 52, Name: "p399_ade_branch_change_request", File: "0052_p399_ade_branch_change_request.sql"},
	{Version: 53, Name: "p401_ade_drop_horizon", File: "0053_p401_ade_drop_horizon.sql"},
	{Version: 54, Name: "r1_ade_branch_review_comments", File: "0054_r1_ade_branch_review_comments.sql"},
	{Version: 55, Name: "p405_ade_plan_backlog", File: "0055_p405_ade_plan_backlog.sql"},
	{Version: 56, Name: "p405_fold_ade_backlog", File: "0056_p405_fold_ade_backlog.sql"},
	{Version: 57, Name: "p407_ade_handovers", File: "0057_p407_ade_handovers.sql"},
	{Version: 58, Name: "p409_ade_task_artifacts", File: "0058_p409_ade_task_artifacts.sql"},
	{Version: 59, Name: "p411_ade_task_status_defaults", File: "0059_p411_ade_task_status_defaults.sql"},
	{Version: 60, Name: "p415_op_log", File: "0060_p415_op_log.sql"},
	{Version: 61, Name: "p441_script_run_restore", File: "0061_p441_script_run_restore.sql"},
	{Version: 62, Name: "p447_secret_values", File: "0062_p447_secret_values.sql"},
}

// All returns every migration in ascending version order.
func All() ([]sqlitex.Migration, error) {
	return sqlitex.LoadMigrations(files, names)
}
