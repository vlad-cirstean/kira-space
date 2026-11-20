import { z } from 'zod';
import { scriptRunRestoreItemSchema, scriptRunRestoreResultSchema } from './scriptRuns';

// Mirrors ade.AgentItem: one agent a quit or crash cut, offered again. `id` is `run:<runId>` or `session:<sessionId>`.
export const sessionRestoreAgentSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  kind: z.enum(['run', 'session']),
  runId: z.string(),
  sessionId: z.string(),
  taskId: z.string(),
  taskTitle: z.string(),
  branchId: z.string(),
  branchLabel: z.string(),
  step: z.string(),
  provider: z.string(),
  mode: z.enum(['headless', 'tui']),
  working: z.boolean(),
  continue: z.enum(['optional', 'required']),
  continueDefault: z.boolean(),
  state: z.enum(['ready', 'unavailable']),
  reason: z.string(),
});
export type SessionRestoreAgent = z.infer<typeof sessionRestoreAgentSchema>;

// Mirrors docker.RestoreState, as far as the dialog reads it.
export const sessionRestoreDockerSchema = /*#__PURE__*/ z.object({
  state: z.enum(['none', 'pending']),
  engineId: z.string(),
  context: z.string(),
  detectedAt: z.number(),
  bootAt: z.number(),
  items: z.array(
    z.object({
      id: z.string(),
      name: z.string(),
      image: z.string(),
      composeProject: z.string(),
      composeService: z.string(),
      status: z.enum(['restorable', 'waiting', 'back', 'gone']),
      error: z.string().optional(),
    }),
  ),
});

// Mirrors sessionrestore.State. `open` says the dialog may still open itself.
export const sessionRestoreStateSchema = /*#__PURE__*/ z.object({
  open: z.boolean(),
  scripts: z.array(scriptRunRestoreItemSchema),
  docker: sessionRestoreDockerSchema,
  agents: z.array(sessionRestoreAgentSchema),
});
export type SessionRestoreState = z.infer<typeof sessionRestoreStateSchema>;

// Mirrors sessionrestore.Args: scripts with the plan hash the person saw, Docker ids, agents with Continue.
export interface SessionRestoreArgs {
  scripts: { runId: string; hash: string }[];
  docker: { engineId: string; ids: string[] } | null;
  agents: { id: string; continue: boolean }[];
}

// Mirrors sessionrestore.Result: one outcome per chosen item. A TUI agent carries the launch to open.
export const sessionRestoreResultSchema = /*#__PURE__*/ z.object({
  scripts: scriptRunRestoreResultSchema.shape.items,
  docker: z
    .object({
      items: z.array(
        z.object({ id: z.string(), name: z.string(), ok: z.boolean(), error: z.string() }),
      ),
    })
    .nullable(),
  agents: z.array(
    z.object({
      id: z.string(),
      launch: z
        .object({
          terminalId: z.string(),
          sessionId: z.string(),
          command: z.string(),
          cwd: z.string(),
        })
        .nullable(),
      runId: z.string(),
      code: z.string(),
      error: z.string(),
    }),
  ),
});
export type SessionRestoreResult = z.infer<typeof sessionRestoreResultSchema>;
