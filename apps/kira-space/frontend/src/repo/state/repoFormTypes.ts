import type { FormEdit, FormProblem } from '@workbench/config/configForm';

// Mirrors internal/repocfg File in the file's own key vocabulary. Run references are opaque here:
// the form only shows which script one names and never edits it.
export interface RepoScriptRefValue {
  script?: string;
}
export interface RepoFileEnvironment {
  name: string;
  deployed_sha_script?: string;
  deployed_sha_run?: RepoScriptRefValue;
}
export interface RepoFileWorktree {
  prepare_script?: string;
  prepare_run?: RepoScriptRefValue;
  prepare_timeout?: string;
  base_path?: string;
}
export interface RepoFileModel {
  path: string;
  nickname?: string;
  integration_branches?: string[];
  environments?: RepoFileEnvironment[];
  worktree?: RepoFileWorktree;
}
export interface RepoFormArgs {
  file: string;
  text: string;
  edits: FormEdit[];
}
export interface RepoFormResult {
  text: string;
  value: RepoFileModel | null;
  problem: FormProblem | null;
}
