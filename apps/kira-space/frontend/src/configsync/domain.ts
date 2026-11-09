// P554: wire shapes of internal/configsync (type only; the bridge trusts them like its siblings).
import type { MessageKey } from '@theme/i18n';

type ConfigSyncModule = {
  name: string;
  mode: string;
  modes: string[];
  units: number;
  deactivated: number;
};

type ConfigSyncProblem = { kind: string; message: string };

export type ConfigSyncStatus = {
  configured: boolean;
  url: string;
  branch: string;
  fetchMinutes: number;
  fetched: string;
  applied: string;
  fetchedAt: string;
  appliedAt: string;
  fetching: boolean;
  problem: ConfigSyncProblem | null;
  changes: number;
  available: boolean;
  showToast: boolean;
  modules: ConfigSyncModule[];
};

export type ConfigSyncKind =
  | 'add'
  | 'modify'
  | 'delete'
  | 'conflict'
  | 'blocked'
  | 'unchanged'
  | 'unmatched';

export type ConfigSyncRow = {
  id: string;
  module: string;
  file: string;
  entry: string;
  kind: ConfigSyncKind;
  deactivated: boolean;
  runsCommands: boolean;
  problem: string;
  unsetSecrets: string[];
  // A repos unit: the local file it merges into, the repository path and the normalized origin.
  target?: string;
  repo?: string;
  origin?: string;
};

type ConfigSyncFileDiff = {
  module: string;
  file: string;
  current: string;
  after: string;
  currentExists: boolean;
  afterExists: boolean;
};

export type ConfigSyncPreview = {
  token: string;
  commit: string;
  rows: ConfigSyncRow[];
  files: ConfigSyncFileDiff[];
};

export type ConfigSyncChoice = 'mine' | 'shared';

export type ConfigSyncApplyResult = {
  applied: number;
  kept: number;
  blocked: number;
  backup: string;
};

export const MODULE_LABEL: Record<string, MessageKey> = {
  connections: 'common.module.connections.label',
  api: 'space.settings.configSync.module.api.label',
  runs: 'space.settings.configSync.module.runs.label',
  workflows: 'space.settings.configSync.module.workflows.label',
  steps: 'space.settings.configSync.module.steps.label',
  agent: 'space.settings.configSync.module.agent.label',
  prompts: 'space.settings.configSync.module.prompts.label',
  repos: 'git.toolbar.repoSettings.tooltip',
};

// The unit key SetDeactivated takes: the file, or file#entry.
export const unitKey = (row: Pick<ConfigSyncRow, 'file' | 'entry'>): string =>
  row.entry === '' ? row.file : `${row.file}#${row.entry}`;
