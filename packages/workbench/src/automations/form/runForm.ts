import type {
  McpChoice,
  RunFile,
  ScriptParam,
  ScriptParamType,
  ScriptSchedule,
  SmartSettings,
} from '@shared/domain/scripts';
import type { FieldSpec } from '../../config/configForm';

// The form edits the file's own keys. The field table names every leaf the form writes; a key not
// listed (and unknown keys inside a list row) is never sent, so it stays in the file.
export const RUN_FIELDS: readonly FieldSpec[] = [
  { path: ['name'] },
  { path: ['command'] },
  { path: ['prompt'] },
  { path: ['collection'], unsetWhenEmpty: true },
  { path: ['color'], unsetWhenEmpty: true },
  { path: ['dir'], unsetWhenEmpty: true },
  { path: ['path'], unsetWhenEmpty: true },
  { path: ['use_ade_dir'] },
  { path: ['params'], unsetWhenEmpty: true },
  { path: ['smart', 'model'] },
  { path: ['smart', 'max_budget_usd'] },
  { path: ['smart', 'no_cost_cap'] },
  { path: ['smart', 'timeout'], unsetWhenEmpty: true },
  { path: ['smart', 'tools'] },
  { path: ['smart', 'bash_patterns'], unsetWhenEmpty: true },
  { path: ['smart', 'mcp'], unsetWhenEmpty: true },
  { path: ['schedule'], presence: true },
  { path: ['schedule', 'cron'] },
  { path: ['schedule', 'timezone'], unsetWhenEmpty: true },
  { path: ['schedule', 'enabled'] },
  { path: ['schedule', 'confirm'] },
  { path: ['schedule', 'timeout'], unsetWhenEmpty: true },
  { path: ['schedule', 'params'], unsetWhenEmpty: true },
];

/** Pointers the Script, Params and Schedule tabs claim; a problem outside them shows on top. */
export const RUN_TAB_POINTERS = {
  script: [
    '/name',
    '/kind',
    '/command',
    '/prompt',
    '/collection',
    '/color',
    '/dir',
    '/path',
    '/use_ade_dir',
    '/smart',
  ],
  params: ['/params'],
  schedule: ['/schedule'],
} as const;

export function paramsOf(file: RunFile): ScriptParam[] {
  return (file.params ?? []).map((p) => ({
    name: p.name,
    type: p.type as ScriptParamType,
    options: [...(p.options ?? [])],
    default: typeof p.default === 'string' ? [p.default] : [...(p.default ?? [])],
    required: p.required ?? false,
    secret: p.secret ?? false,
  }));
}

export function fileParamsOf(params: readonly ScriptParam[]): NonNullable<RunFile['params']> {
  return params.map((p) => {
    const row: NonNullable<RunFile['params']>[number] = { name: p.name, type: p.type };
    if (p.options.length > 0) row.options = [...p.options];
    if (p.default.length > 0)
      row.default =
        p.default.length === 1 && p.type !== 'multiselect' ? p.default[0] : [...p.default];
    if (p.required) row.required = true;
    if (p.secret) row.secret = true;
    return row;
  });
}

export function smartOf(file: RunFile, fallback: SmartSettings): SmartSettings {
  const s = file.smart;
  if (!s) return fallback;
  return {
    model: s.model ?? fallback.model,
    maxBudgetUsd: s.max_budget_usd ?? 0,
    noCostCap: s.no_cost_cap ?? false,
    timeout: s.timeout ?? '',
    tools: [...(s.tools ?? [])],
    bashPatterns: [...(s.bash_patterns ?? [])],
    mcp: (s.mcp ?? []).map(
      (c): McpChoice => ({
        server: c.server,
        allTools: c.all_tools ?? false,
        tools: [...(c.tools ?? [])],
      }),
    ),
  };
}

export function fileSmartOf(s: SmartSettings): NonNullable<RunFile['smart']> {
  return {
    model: s.model,
    max_budget_usd: s.maxBudgetUsd,
    no_cost_cap: s.noCostCap,
    timeout: s.timeout,
    tools: [...s.tools],
    bash_patterns: [...s.bashPatterns],
    mcp: s.mcp.map((c) => ({ server: c.server, all_tools: c.allTools, tools: [...c.tools] })),
  };
}

export function scheduleOf(file: RunFile): ScriptSchedule | null {
  const s = file.schedule;
  if (!s) return null;
  const params: Record<string, string[]> = {};
  for (const [k, v] of Object.entries(s.params ?? {}))
    params[k] = typeof v === 'string' ? [v] : [...v];
  return {
    cron: s.cron,
    timezone: s.timezone ?? '',
    enabled: s.enabled,
    confirm: s.confirm,
    timeout: s.timeout ?? '',
    params,
    taskId: '',
    branchId: '',
  };
}

export function fileScheduleOf(s: ScriptSchedule): NonNullable<RunFile['schedule']> {
  return {
    cron: s.cron,
    timezone: s.timezone,
    enabled: s.enabled,
    confirm: s.confirm,
    timeout: s.timeout,
    params: Object.fromEntries(
      Object.entries(s.params).map(([k, v]) => [k, v.length === 1 ? (v[0] as string) : [...v]]),
    ),
  };
}
