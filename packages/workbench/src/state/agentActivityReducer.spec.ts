// The one earned unit test for the frontend half: reduceAgentActivity is a
// decision structure fed by an out-of-order external producer (each hook is its own `curl`
// process, so a PostToolUse can legitimately arrive before its own PreToolUse), with several
// interacting rules — exactly what CLAUDE.md's narrow unit-test bar names. Pure: (prev, event,
// now) =&gt; next; createAgentSessionsStore applies it, this spec calls it directly.
//
// Colocated with the reducer (packages/git-core/src's own precedent) — a plain
// static import now, since agentActivity.ts reaches no bridge binding at all.
//
// `now` is injected and every case below passes a fixed NOW, so `at` is
// deterministic; new cases cover UserPromptSubmit, the wakeArmed/'waiting' rule, idle_prompt/
// auth_success leaving phase unchanged, and PostToolUse clearing 'attention'.
import { describe, expect, test } from 'bun:test';
import type { AgentEvent } from '@shared/domain/agent';
import { MAX_RUNNING_TOOLS, reduceAgentActivity } from './agentActivity';

const NOW = 1_700_000_000_000;

function event(partial: Partial<AgentEvent> & { event: string }): AgentEvent {
  return {
    terminalId: 't1',
    sessionId: 's1',
    cwd: '/repo',
    toolName: '',
    toolUseId: '',
    notificationType: '',
    message: '',
    source: '',
    reason: '',
    lastAssistantMessage: '',
    agentId: '',
    background: 0,
    ...partial,
  };
}

describe('agentActivity — reduceAgentActivity (P86 §13, P129 Part 1 §4.8)', () => {
  test('1. SessionStart on an existing entry resets rather than merges', () => {
    const busy = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a' }),
      NOW,
    );
    expect(busy.runningTools).toEqual(['a']);

    const restarted = reduceAgentActivity(
      busy,
      event({ event: 'SessionStart', sessionId: 's2' }),
      NOW + 1,
    );
    expect(restarted).toEqual({
      phase: 'idle',
      runningTools: [],
      toolName: null,
      message: null,
      sessionId: 's2',
      wakeArmed: false,
      at: NOW + 1,
      background: 0,
    });
  });

  test('2. PreToolUse then PostToolUse for the same tool_use_id leaves runningTools empty, phase still working', () => {
    const started = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'Bash' }),
      NOW,
    );
    const finished = reduceAgentActivity(
      started,
      event({ event: 'PostToolUse', toolUseId: 'a' }),
      NOW,
    );
    expect(finished.runningTools).toEqual([]);
    expect(finished.phase).toBe('working');
  });

  test('3. PostToolUse arriving before its own PreToolUse leaves the set empty, never negative', () => {
    const result = reduceAgentActivity(
      undefined,
      event({ event: 'PostToolUse', toolUseId: 'a' }),
      NOW,
    );
    expect(result.runningTools).toEqual([]);
  });

  test('4. an unmatched PostToolUse is a no-op', () => {
    const started = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'Bash' }),
      NOW,
    );
    const stillRunning = reduceAgentActivity(
      started,
      event({ event: 'PostToolUse', toolUseId: 'b' }),
      NOW,
    );
    expect(stillRunning.runningTools).toEqual(['a']);
  });

  test('5. Notification during a tool call sets attention and keeps runningTools; the following PostToolUse still matches', () => {
    const started = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'Bash' }),
      NOW,
    );
    const waiting = reduceAgentActivity(
      started,
      event({ event: 'Notification', message: 'May I run this?' }),
      NOW,
    );
    expect(waiting.phase).toBe('attention');
    expect(waiting.message).toBe('May I run this?');
    expect(waiting.runningTools).toEqual(['a']);

    const finished = reduceAgentActivity(
      waiting,
      event({ event: 'PostToolUse', toolUseId: 'a' }),
      NOW,
    );
    expect(finished.runningTools).toEqual([]);
  });

  test('6. Stop clears the set, the message and the tool name', () => {
    const started = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'Bash' }),
      NOW,
    );
    const waiting = reduceAgentActivity(
      started,
      event({ event: 'Notification', message: 'hi' }),
      NOW,
    );
    const stopped = reduceAgentActivity(waiting, event({ event: 'Stop' }), NOW);
    expect(stopped.phase).toBe('idle');
    expect(stopped.runningTools).toEqual([]);
    expect(stopped.message).toBeNull();
    expect(stopped.toolName).toBeNull();
    expect(stopped.wakeArmed).toBe(false);
  });

  test('7. runningTools past MAX_RUNNING_TOOLS drops the oldest, not the newest', () => {
    let activity = reduceAgentActivity(undefined, event({ event: 'SessionStart' }), NOW);
    for (let i = 0; i < MAX_RUNNING_TOOLS + 5; i++) {
      activity = reduceAgentActivity(
        activity,
        event({ event: 'PreToolUse', toolUseId: `t${i}` }),
        NOW,
      );
    }
    expect(activity.runningTools.length).toBe(MAX_RUNNING_TOOLS);
    // The oldest five (t0..t4) are gone; the newest is still present.
    expect(activity.runningTools).not.toContain('t0');
    expect(activity.runningTools).not.toContain('t4');
    expect(activity.runningTools).toContain('t5');
    expect(activity.runningTools[activity.runningTools.length - 1]).toBe(
      `t${MAX_RUNNING_TOOLS + 4}`,
    );
  });

  test('8. UserPromptSubmit sets working, clears message and wakeArmed', () => {
    const attention = reduceAgentActivity(
      undefined,
      event({ event: 'Notification', message: 'hi' }),
      NOW,
    );
    expect(attention.phase).toBe('attention');

    const submitted = reduceAgentActivity(attention, event({ event: 'UserPromptSubmit' }), NOW + 1);
    expect(submitted.phase).toBe('working');
    expect(submitted.message).toBeNull();
    expect(submitted.wakeArmed).toBe(false);
    expect(submitted.at).toBe(NOW + 1);
  });

  test('9. a Monitor tool call arms wakeArmed, and Stop turns that into waiting, not idle', () => {
    const monitoring = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'Monitor' }),
      NOW,
    );
    expect(monitoring.wakeArmed).toBe(true);

    const stopped = reduceAgentActivity(monitoring, event({ event: 'Stop' }), NOW + 1);
    expect(stopped.phase).toBe('waiting');
    expect(stopped.wakeArmed).toBe(false);
  });

  test('10. after waiting, the next PreToolUse (the monitor firing) moves back to working', () => {
    const monitoring = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'ScheduleWakeup' }),
      NOW,
    );
    const stopped = reduceAgentActivity(monitoring, event({ event: 'Stop' }), NOW);
    expect(stopped.phase).toBe('waiting');

    const resumed = reduceAgentActivity(
      stopped,
      event({ event: 'PreToolUse', toolUseId: 'b', toolName: 'Bash' }),
      NOW + 1,
    );
    expect(resumed.phase).toBe('working');
  });

  test('11. Notification with notificationType idle_prompt or auth_success leaves phase unchanged', () => {
    const idle = reduceAgentActivity(undefined, event({ event: 'SessionStart' }), NOW);
    expect(idle.phase).toBe('idle');

    const stillIdle = reduceAgentActivity(
      idle,
      event({ event: 'Notification', notificationType: 'idle_prompt', message: 'ignored' }),
      NOW + 1,
    );
    expect(stillIdle.phase).toBe('idle');
    expect(stillIdle.at).toBe(NOW + 1);

    const working = reduceAgentActivity(
      idle,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'Bash' }),
      NOW,
    );
    const stillWorking = reduceAgentActivity(
      working,
      event({ event: 'Notification', notificationType: 'auth_success', message: 'ignored' }),
      NOW + 1,
    );
    expect(stillWorking.phase).toBe('working');
  });

  test('12. PostToolUse turns attention back into working', () => {
    const started = reduceAgentActivity(
      undefined,
      event({ event: 'PreToolUse', toolUseId: 'a', toolName: 'Bash' }),
      NOW,
    );
    const attention = reduceAgentActivity(
      started,
      event({ event: 'Notification', message: 'May I run this?' }),
      NOW,
    );
    expect(attention.phase).toBe('attention');

    const finished = reduceAgentActivity(
      attention,
      event({ event: 'PostToolUse', toolUseId: 'a' }),
      NOW + 1,
    );
    expect(finished.phase).toBe('working');
    expect(finished.at).toBe(NOW + 1);
  });

  test('13. recorded background sequence: Stop sets the count, subagent hooks never move phase', () => {
    const lines: Array<Partial<AgentEvent> & { event: string }> = [
      { event: 'SessionStart' },
      { event: 'UserPromptSubmit' },
      { event: 'Stop', background: 2 },
      { event: 'PreToolUse', toolUseId: 'x', toolName: 'Bash', agentId: 'a1' },
    ];
    let a = reduceAgentActivity(undefined, event(lines[0]), NOW);
    a = reduceAgentActivity(a, event(lines[1]), NOW);
    a = reduceAgentActivity(a, event(lines[2]), NOW);
    expect(a.phase).toBe('idle');
    expect(a.background).toBe(2);

    a = reduceAgentActivity(a, event(lines[3]), NOW + 1);
    expect(a.phase).toBe('idle');
    expect(a.runningTools).toEqual([]);
    expect(a.toolName).toBeNull();

    a = reduceAgentActivity(a, event({ event: 'SubagentStop', agentId: 'a1', background: 1 }), NOW);
    expect(a.phase).toBe('idle');
    expect(a.background).toBe(1);

    a = reduceAgentActivity(a, event({ event: 'UserPromptSubmit' }), NOW);
    a = reduceAgentActivity(a, event({ event: 'Stop', background: 0 }), NOW);
    expect(a.phase).toBe('idle');
    expect(a.background).toBe(0);
  });

  test('14. subagent permission prompt enters attention, its PostToolUse leaves it', () => {
    let a = reduceAgentActivity(undefined, event({ event: 'UserPromptSubmit' }), NOW);
    a = reduceAgentActivity(
      a,
      event({
        event: 'Notification',
        agentId: 'a1',
        notificationType: 'permission_prompt',
        message: 'ok?',
      }),
      NOW + 1,
    );
    expect(a.phase).toBe('attention');
    expect(a.message).toBe('ok?');

    a = reduceAgentActivity(
      a,
      event({ event: 'PostToolUse', toolUseId: 'x', agentId: 'a1' }),
      NOW + 2,
    );
    expect(a.phase).toBe('working');
    expect(a.runningTools).toEqual([]);

    a = reduceAgentActivity(
      a,
      event({ event: 'Notification', agentId: 'a1', notificationType: 'idle_prompt' }),
      NOW + 3,
    );
    expect(a.phase).toBe('working');
  });
});
