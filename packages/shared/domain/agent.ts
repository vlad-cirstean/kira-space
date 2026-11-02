import { z } from 'zod';

// The TypeScript mirrors of Go's terminal.AgentSession/harness.AgentEvent wire shapes.
// grpc-history.ts's own precedent — types, not guards: neither crosses a storage boundary or gets
// restored, so nothing here earns a Zod-parse boundary. control.ts `trust<T>()`s them like every
// other bound result.

// terminal.AgentSession's own wire projection (bridge/terminal.go's AgentSessionWire) — one live
// Claude Code tab, across every window (§11: the count's authority is Go).
export interface AgentSession {
  terminalId: string;
  cwd: string;
}

// ChannelAgentSessions' own payload (bridge/terminal.go's AgentSessionsEvent) — a list, not a bare
// count: a bare count would leave the status-bar tooltip unable to say *which* sessions (§11).
export interface AgentSessionsEvent {
  sessions: AgentSession[];
}

// harness.AgentEvent's own wire projection, ChannelAgentEvent's payload (§6) — one hook firing for
// one tab. Every field the listener kept; everything else (tool_input, tool_response,
// transcript_path, ...) was already dropped server-side and never reaches here.
export interface AgentEvent {
  terminalId: string;
  // hook_event_name: SessionStart, SessionEnd, PreToolUse, PostToolUse, Notification, Stop,
  // UserPromptSubmit (P129 Part 1 §4.8: a new user turn is the one reliable "working" start for a
  // text-only reply).
  event: string;
  sessionId: string;
  cwd: string;
  toolName: string;
  toolUseId: string;
  notificationType: string;
  message: string; // bounded server-side (§6.1)
  source: string; // SessionStart
  reason: string; // SessionEnd
  lastAssistantMessage: string; // Stop, bounded server-side
}

// Reducer output — one Claude Code tab's current activity, derived from the AgentEvent
// stream by state/agentActivity.ts's reduceAgentActivity. 'waiting': a session that armed a wake
// tool (Monitor, ScheduleWakeup) before its own Stop is "waiting on monitor", not plain idle
// (design mapping done in the UI, not here).
export type AgentPhase = 'idle' | 'working' | 'attention' | 'waiting';

export interface AgentActivity {
  phase: AgentPhase;
  runningTools: string[]; // tool_use_id, insertion-ordered, capped at MAX_RUNNING_TOOLS
  toolName: string | null; // most recent PreToolUse's tool, for the tooltip
  message: string | null; // Notification text, when phase is 'attention'
  sessionId: string | null;
  // wakeArmed is P129 Part 1's own addition: true once a wake tool (Monitor, ScheduleWakeup) has
  // run during the current turn, cleared by the next UserPromptSubmit or Stop. Turns 'idle' into
  // 'waiting' on Stop.
  wakeArmed: boolean;
  // at is P129 Part 1's own addition: receipt time (ms) of the event that produced this activity —
  // every event updates it, including one that leaves phase unchanged.
  at: number;
}

// agentsvc.Info: what the UI needs to name and configure one agent provider.
export const agentInfoSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  label: z.string(),
  productLabel: z.string(),
  command: z.string(),
  headlessLabel: z.string(),
  models: z.array(z.object({ id: z.string(), label: z.string() })),
  defaultModel: z.string(),
  hooks: z.boolean(),
  usage: z.boolean(),
});
export type AgentInfo = z.infer<typeof agentInfoSchema>;

// agentsvc.Entry: one registered provider and whether its CLI is installed.
export const agentEntrySchema = /*#__PURE__*/ agentInfoSchema.extend({ available: z.boolean() });
export type AgentEntry = z.infer<typeof agentEntrySchema>;
