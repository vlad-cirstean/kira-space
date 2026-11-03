import { z } from 'zod';
import { paletteColorSchema } from './color';
import { secretStorageStatusSchema } from './secrets';

// P85 §9.2: zod schemas mirroring internal/scripts — mask.ts's own
// Fields/full split (maskRuleFieldsSchema/maskRuleSchema).

// Where a script runs: the app's automations folder, or a picked folder.
export const scriptDirModeSchema = /*#__PURE__*/ z.enum(['kira', 'fixed']);
export type ScriptDirMode = z.infer<typeof scriptDirModeSchema>;

// A normal script runs a shell command; a smart script runs its text as a prompt to headless Claude.
export const scriptKindSchema = /*#__PURE__*/ z.enum(['script', 'smart']);
export type ScriptKind = z.infer<typeof scriptKindSchema>;

// The Claude built-ins a smart script may tick, and the ones ticked for a new script.
export const BUILTIN_TOOLS = [
  'Read',
  'Grep',
  'Glob',
  'Edit',
  'Write',
  'NotebookEdit',
  'Bash',
] as const;
export const DEFAULT_TOOLS: readonly string[] = ['Read', 'Grep', 'Glob'];
// Variables a workflow run has; reserved as param names.
export const BUILTIN_VARS = ['task', 'jira', 'repo', 'branch', 'base', 'worktree'] as const;
export const DEFAULT_SMART = { maxBudgetUsd: 1, noCostCap: false, timeout: '15m' } as const;

export const scriptParamTypeSchema = /*#__PURE__*/ z.enum(['text', 'select', 'multiselect']);
export type ScriptParamType = z.infer<typeof scriptParamTypeSchema>;

// Mirrors scripts.Param.
export const scriptParamSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  type: scriptParamTypeSchema,
  options: z.array(z.string()),
  default: z.array(z.string()),
  required: z.boolean(),
  secret: z.boolean(),
});
export type ScriptParam = z.infer<typeof scriptParamSchema>;

// Mirrors scripts.MCPChoice.
export const mcpChoiceSchema = /*#__PURE__*/ z.object({
  server: z.string(),
  allTools: z.boolean(),
  tools: z.array(z.string()),
});
export type McpChoice = z.infer<typeof mcpChoiceSchema>;

// Mirrors scripts.Smart.
export const smartSettingsSchema = /*#__PURE__*/ z.object({
  model: z.string(),
  maxBudgetUsd: z.number(),
  // No cost cap: maxBudgetUsd is 0. Needed to run on a provider that cannot cap cost.
  noCostCap: z.boolean(),
  timeout: z.string(),
  tools: z.array(z.string()),
  bashPatterns: z.array(z.string()),
  mcp: z.array(mcpChoiceSchema),
});
export type SmartSettings = z.infer<typeof smartSettingsSchema>;

// Mirrors scripts.Schedule: makes a script recurring.
export const scriptScheduleSchema = /*#__PURE__*/ z.object({
  cron: z.string(),
  timezone: z.string(),
  enabled: z.boolean(),
  // Ask before each run.
  confirm: z.boolean(),
  // A normal script's headless run limit; '' = the default.
  timeout: z.string(),
  // Fixed non-secret param values.
  params: z.record(z.string(), z.array(z.string())),
  // Kira Space: the ADE task and branch a run targets; '' for none.
  taskId: z.string(),
  branchId: z.string(),
});
export type ScriptSchedule = z.infer<typeof scriptScheduleSchema>;

// customScriptSchema mirrors scripts.CustomScript — one run: its run file's config plus state. The
// file is the authority; the Go validator checks it.
export const customScriptSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  name: z.string(),
  // The shell command, or the prompt of a smart script.
  command: z.string(),
  kind: scriptKindSchema,
  params: z.array(scriptParamSchema),
  // null for a normal script.
  smart: smartSettingsSchema.nullable(),
  // null for a script that is not recurring.
  schedule: scriptScheduleSchema.nullable(),
  dirMode: scriptDirModeSchema,
  // The picked folder when dirMode is 'fixed'; '' otherwise.
  workingDir: z.string(),
  color: paletteColorSchema,
  // The panel group; '' is ungrouped.
  collection: z.string(),
  // Kira Space: run in the ADE task's worktree when started from ADE.
  useAdeDir: z.boolean(),
  sortOrder: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
  // Secret params that have a saved value; never the value itself.
  savedSecrets: z.array(z.string()),
});
export type CustomScript = z.infer<typeof customScriptSchema>;

// What Create takes: the run's config, the part a file holds.
export type CustomScriptFields = Omit<
  CustomScript,
  'id' | 'sortOrder' | 'createdAt' | 'updatedAt' | 'savedSecrets'
>;

// Saved secret values by param name, as SetSecrets takes them: a string saves, null clears, an
// absent name keeps. Write-only: no read ever returns a value.
export const secretValuesSchema = /*#__PURE__*/ z.record(z.string(), z.string().nullable());
export type SecretValues = z.infer<typeof secretValuesSchema>;

// scriptCollectionSchema mirrors scripts.Collection — one panel group's order; the name is the key.
export const scriptCollectionSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  sortOrder: z.number(),
});
export type ScriptCollection = z.infer<typeof scriptCollectionSchema>;

// scriptFileProblemSchema mirrors scripts.FileProblem — one invalid run file.
export const scriptFileProblemSchema = /*#__PURE__*/ z.object({
  file: z.string(),
  // '' when the file name is not a valid id.
  id: z.string(),
  line: z.number(),
  message: z.string(),
  // A previous valid version keeps running.
  lastGood: z.boolean(),
});
export type ScriptFileProblem = z.infer<typeof scriptFileProblemSchema>;

// scriptsSnapshotSchema mirrors scripts.Snapshot — what List answers and every
// change broadcasts.
export const scriptsSnapshotSchema = /*#__PURE__*/ z.object({
  // The runs folder.
  dir: z.string(),
  collections: z.array(scriptCollectionSchema),
  scripts: z.array(customScriptSchema),
  problems: z.array(scriptFileProblemSchema),
  // Fixed for the process; off means a secret param's value cannot be saved.
  secretStorage: secretStorageStatusSchema,
});
export type ScriptsSnapshot = z.infer<typeof scriptsSnapshotSchema>;

// scriptFileViewSchema mirrors scripts.FileView — one run file's text and state.
export const scriptFileViewSchema = /*#__PURE__*/ z.object({
  file: z.string(),
  id: z.string(),
  path: z.string(),
  text: z.string(),
  hash: z.string(),
  problem: z.object({ line: z.number(), message: z.string() }).nullable(),
  lastGood: z.boolean(),
  editable: z.boolean(),
});
export type ScriptFileView = z.infer<typeof scriptFileViewSchema>;

// scriptDirSchema mirrors scripts.Dir — where a saved script runs, and why it cannot.
export const scriptDirSchema = /*#__PURE__*/ z.object({
  path: z.string(),
  mode: /*#__PURE__*/ z.enum(['kira', 'fixed', 'worktree']),
  base: z.string(),
  blocker: z.string(),
  // 'repo · branch' of a worktree folder; pending = the worktree is created on Run.
  branch: z.string(),
  pending: z.boolean(),
});
export type ScriptDir = z.infer<typeof scriptDirSchema>;

// scriptMcpServerSchema mirrors harness.UserServer — a server `claude mcp list` reports, never its
// args, env or headers. status is claude's own health report.
export const scriptMcpServerSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  transport: z.string(),
  target: z.string(),
  // '' for a server from Kira's agent settings: Kira never health-checks it.
  status: z.enum(['', 'connected', 'failed', 'needsAuth', 'pending', 'unknown']),
});
export type ScriptMcpServer = z.infer<typeof scriptMcpServerSchema>;

// scriptMcpServerListSchema is the picker's list and its source: Kira's agent settings (app) when
// hermetic, else the agent CLI's own list (agent).
export const scriptMcpServerListSchema = /*#__PURE__*/ z.object({
  source: z.enum(['app', 'agent']),
  servers: z.array(scriptMcpServerSchema),
});
export type ScriptMcpServerList = z.infer<typeof scriptMcpServerListSchema>;
