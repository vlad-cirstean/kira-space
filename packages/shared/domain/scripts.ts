import { z } from 'zod';
import { paletteColorSchema } from './color';
import { secretStorageStatusSchema } from './secrets';

// P85 §9.2: zod schemas mirroring internal/scripts — mask.ts's own
// Fields/full split (maskRuleFieldsSchema/maskRuleSchema).

// Where a script runs: the app's automations folder, a picked folder, or (legacy rows only) $HOME.
export const scriptDirModeSchema = /*#__PURE__*/ z.enum(['kira', 'fixed', 'home']);
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
export const DEFAULT_SMART = { maxBudgetUsd: 1, timeout: '15m' } as const;

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

// customScriptFieldsSchema mirrors scripts.CustomScriptFields — what the Create/Update bound calls
// accept. The Go check (scripts.CustomScriptFields.Validate) is the authority; this is
// only the dialog's own affordance for disabling Add before a round trip.
export const customScriptFieldsSchema = /*#__PURE__*/ z.object({
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
  // null = ungrouped; the panel groups rows under their collection.
  collectionId: z.string().nullable(),
  // Kira Space: run in the ADE task's worktree when started from ADE. Studio stores it, never shows it.
  useAdeDir: z.boolean(),
});
export type CustomScriptFields = z.infer<typeof customScriptFieldsSchema>;

// customScriptSchema mirrors model.CustomScript — one custom_scripts row.
export const customScriptSchema = /*#__PURE__*/ customScriptFieldsSchema.extend({
  id: z.string(),
  sortOrder: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
  // Secret params that have a saved value; never the value itself.
  savedSecrets: z.array(z.string()),
});
export type CustomScript = z.infer<typeof customScriptSchema>;

// Saved secret values by param name, as Create/Update take them: a string saves, null clears, an
// absent name keeps. Write-only: no read ever returns a value.
export const secretValuesSchema = /*#__PURE__*/ z.record(z.string(), z.string().nullable());
export type SecretValues = z.infer<typeof secretValuesSchema>;

// scriptCollectionSchema mirrors scripts.Collection — one custom_script_collections row.
export const scriptCollectionSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  name: z.string(),
  sortOrder: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type ScriptCollection = z.infer<typeof scriptCollectionSchema>;

// scriptsSnapshotSchema mirrors scripts.Snapshot — what List answers and every
// mutation broadcasts.
export const scriptsSnapshotSchema = /*#__PURE__*/ z.object({
  collections: z.array(scriptCollectionSchema),
  scripts: z.array(customScriptSchema),
  // Fixed for the process; off means a secret param's value cannot be saved.
  secretStorage: secretStorageStatusSchema,
});
export type ScriptsSnapshot = z.infer<typeof scriptsSnapshotSchema>;

// scriptDirSchema mirrors scripts.Dir — where a saved script runs, and why it cannot.
export const scriptDirSchema = /*#__PURE__*/ z.object({
  path: z.string(),
  mode: /*#__PURE__*/ z.enum(['kira', 'fixed', 'home', 'worktree']),
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
  status: z.enum(['connected', 'failed', 'needsAuth', 'pending', 'unknown']),
});
export type ScriptMcpServer = z.infer<typeof scriptMcpServerSchema>;
