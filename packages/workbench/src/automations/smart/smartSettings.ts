import {
  BUILTIN_TOOLS,
  BUILTIN_VARS,
  DEFAULT_SMART,
  DEFAULT_TOOLS,
  type McpChoice,
  type SmartSettings,
} from '@shared/domain/scripts';

/** A new smart script's settings; `model` is the active provider's default alias. */
export function defaultSmart(model: string): SmartSettings {
  return {
    model,
    maxBudgetUsd: DEFAULT_SMART.maxBudgetUsd,
    noCostCap: DEFAULT_SMART.noCostCap,
    timeout: DEFAULT_SMART.timeout,
    tools: [...DEFAULT_TOOLS],
    bashPatterns: [],
    mcp: [],
  };
}

/** The ticked built-ins in the order the picker lists them. */
export function orderedTools(tools: readonly string[]): string[] {
  return BUILTIN_TOOLS.filter((t) => tools.includes(t));
}

/** The `--allowedTools` words a smart script grants, as the run will pass them (internal/scripts
 *  ToolArgs); the run dialog shows the server's own answer. */
export function allowedToolsOf(s: SmartSettings): string[] {
  const out: string[] = [];
  for (const t of orderedTools(s.tools)) {
    if (t === 'Bash' && s.bashPatterns.length > 0) {
      for (const p of s.bashPatterns) out.push(`Bash(${p})`);
    } else out.push(t);
  }
  for (const c of s.mcp) {
    if (c.allTools) out.push(`mcp__${c.server}`);
    else for (const t of c.tools) out.push(`mcp__${c.server}__${t}`);
  }
  return out;
}

const VAR_RE = /\{([A-Za-z_][A-Za-z0-9_]*)\}/g;

/** Distinct `{name}` placeholders of a prompt, in order of first use. */
export function varsUsed(prompt: string): string[] {
  const seen: string[] = [];
  for (const m of prompt.matchAll(VAR_RE)) {
    const name = m[1] ?? '';
    if (!seen.includes(name)) seen.push(name);
  }
  return seen;
}

export function isBuiltinVar(name: string): boolean {
  return (BUILTIN_VARS as readonly string[]).includes(name);
}

export function withMcp(
  mcp: readonly McpChoice[],
  server: string,
  tools: string[],
  allTools: boolean,
): McpChoice[] {
  const rest = mcp.filter((c) => c.server !== server);
  return [...rest, { server, allTools, tools: allTools ? [] : tools }];
}
