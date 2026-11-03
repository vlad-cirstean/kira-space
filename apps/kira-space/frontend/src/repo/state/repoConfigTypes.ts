// Mirrors internal/repocfg Service views and args.
export interface RepoConfigProblem {
  line: number;
  message: string;
}
export interface RepoConfigItem {
  file: string;
  path: string;
  problem: RepoConfigProblem | null;
  lastGood: boolean;
  repo: string /* code repo id; '' = no registered repo has the path */;
}
export interface RepoConfigList {
  dir: string;
  files: RepoConfigItem[];
}
export interface RepoConfigFileArgs {
  path: string;
  file: string;
}
export interface RepoConfigFile {
  file: string /* '' = the repo has none yet; text is a template */;
  filePath: string;
  path: string;
  text: string;
  hash: string /* identifies text; Save sends it back as baseHash */;
  exists: boolean;
  problem: RepoConfigProblem | null;
  lastGood: boolean;
  editable: boolean;
}
export interface RepoConfigSaveArgs {
  path: string;
  file: string;
  text: string;
  baseHash: string;
}
