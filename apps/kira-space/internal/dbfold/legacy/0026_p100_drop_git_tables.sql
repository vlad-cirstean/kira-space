-- P100 Part 1: the git module (and the native code-viewing workspace it carried) moved to
-- apps/kira-space. Nothing in this app reads or writes these three tables any more.
DROP TABLE git_repo_settings;
DROP TABLE git_clients;
DROP TABLE code_repos;

-- tabs.workspace_id (0018_c5_code_repos.sql) is left in place, deliberately: every remaining tab
-- parses as workspace_id IS NULL (code_repos, its only foreign referent, is gone), so dropping the
-- column would rewrite the whole tabs table for zero behavioural gain -- model.NormalizeMode-style
-- degradation already applies here too, since nothing reads a non-NULL value once code_repos is
-- gone.
