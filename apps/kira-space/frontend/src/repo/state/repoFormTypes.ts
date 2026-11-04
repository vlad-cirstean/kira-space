import type { FormEdit, FormProblem } from '@workbench/config/configForm';

// Mirrors internal/repocfg File in the file's own key vocabulary. A param holds one value as a
// string, several as a list (model.ParamValues).
export type RefParamValue = string | string[];
export interface RepoScriptRefValue {
  script: string;
  params?: Record<string, RefParamValue>;
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
