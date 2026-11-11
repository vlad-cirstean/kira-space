import type { AgentActivity, AgentEvent } from '@shared/domain/agent';

export const MAX_RUNNING_TOOLS = 64;

// WAKE_TOOLS is the set — a PreToolUse naming one of these arms `wakeArmed`, which turns a
// following Stop into 'waiting' instead of 'idle'. A `Bash` background run cannot arm it:
// `tool_input` is never decoded (the privacy rule), so a backgrounded watcher script is
// indistinguishable from any other Bash call here (docs/ARCHITECTURE.md's own known limitation).
const WAKE_TOOLS = new Set(['Monitor', 'ScheduleWakeup']);

function emptyActivity(sessionId: string | null, now: number): AgentActivity {
  return {
    phase: 'idle',
    runningTools: [],
    toolName: null,
    message: null,
    sessionId,
    wakeArmed: false,
    at: now,
    background: 0,
  };
}

function reduceNotification(base: AgentActivity, event: AgentEvent, now: number): AgentActivity {
  // idle_prompt/auth_success: an idle session is not "needs input" (design §1) — phase stays
  // put and message is not overwritten with text that was never a request for input.
  if (event.notificationType === 'idle_prompt' || event.notificationType === 'auth_success') {
    return { ...base, at: now };
  }
  // Does not clear runningTools — a permission prompt arrives *during* a tool call; losing
  // the set here would leave the following PostToolUse unmatched too.
  return { ...base, phase: 'attention', message: event.message || null, at: now };
}

// The one piece here with real interacting rules, fed by an out-of-order external
// producer — each hook is its own `curl` process, so a PostToolUse can legitimately arrive before
// its own PreToolUse. Pure: (prev, event, now) => next — `now` is injected so
// this stays pure; createAgentSessionsStore passes Date.now(). createAgentSessionsStore applies it
// below; the unit test (agentActivityReducer.spec.ts, colocated) calls it directly.
export function reduceAgentActivity(
  prev: AgentActivity | undefined,
  event: AgentEvent,
  now: number,
): AgentActivity {
  const base = prev ?? emptyActivity(event.sessionId || null, now);
  if (event.agentId !== '') {
    // A subagent's own hooks move the main session's phase only for what needs the user: a
    // non-idle Notification (a permission prompt raised in the subagent) enters 'attention', and
    // a PostToolUse leaves it. SubagentStop refreshes the background count.
    if (event.event === 'SubagentStop') return { ...base, background: event.background, at: now };
    if (event.event === 'Notification') return reduceNotification(base, event, now);
    if (event.event === 'PostToolUse' && base.phase === 'attention') {
      return { ...base, phase: 'working', at: now };
    }
    return { ...base, at: now };
  }
  switch (event.event) {
    case 'SessionStart':
      // Resets rather than merges — a user who exits `claude` and reruns it in the same tab
      // starts clean.
      return emptyActivity(event.sessionId || null, now);
    case 'UserPromptSubmit':
      // A new user turn is the one reliable "working" start for a text-only reply (no tool call
      // follows a plain answer) — and it always ends whatever wake arming the previous turn set.
      return { ...base, phase: 'working', message: null, wakeArmed: false, at: now };
    case 'PreToolUse': {
      const runningTools = base.runningTools.includes(event.toolUseId)
        ? base.runningTools
        : [...base.runningTools, event.toolUseId].slice(-MAX_RUNNING_TOOLS);
      const wakeArmed = base.wakeArmed || WAKE_TOOLS.has(event.toolName);
      return {
        ...base,
        phase: 'working',
        runningTools,
        toolName: event.toolName,
        wakeArmed,
        at: now,
      };
    }
    case 'PostToolUse': {
      // Pairing is by tool_use_id, never a depth counter — a set is order-insensitive; a counter
      // would go negative and strand the phase. An unmatched id is a no-op, not an error.
      const runningTools = base.runningTools.filter((id) => id !== event.toolUseId);
      // The prompt that caused 'attention' was answered and the tool ran — back to 'working'.
      const phase = base.phase === 'attention' ? 'working' : base.phase;
      return { ...base, phase, runningTools, at: now };
    }
    case 'Notification':
      return reduceNotification(base, event, now);
    case 'Stop': {
      // wakeArmed carries a session into 'waiting' (waiting on monitor) instead of 'idle' — the
      // one signal this reducer has that the session is expecting an external event, not the user.
      const phase = base.wakeArmed ? 'waiting' : 'idle';
      // Clears everything else (P86 §13 rule 4), which is what heals a session that lost a
      // PostToolUse to the listener's own bounds (internal/harness/claude/hooks/http.go §6.1).
      return {
        ...base,
        phase,
        runningTools: [],
        toolName: null,
        message: null,
        wakeArmed: false,
        background: event.background,
        at: now,
      };
    }
    case 'SessionEnd':
      return { ...base, background: 0, at: now };
    default:
      return { ...base, at: now };
  }
}
