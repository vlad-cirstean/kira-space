import type { ShortcutId } from '@shared/domain/shortcuts';
import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import type { Settings, SettingsPatch } from '../../../state/settingsDomain';
import type { CardModel } from '../plan/usePlanModel';
import type { Plan, Run, Session, TaskStatusDef } from '../wire';
import type { BranchGraph } from './branchGraph';
import { type Calendar, isWeekend, LATER, nextWork, offsetToIso } from './calendar';
import type { ChangeBaseChoice } from './changeBase';
import { type RebaseAct, rebaseActs } from './rebaseActions';
import type { ReviewChoice } from './reviewCode';
import { nextStepId } from './stepBlocks';

// ---- Task context menu
// Task context menu (P198): which entries a task shows and what each does. Pure; `useTaskMenu` maps
// a command to its call.

export type TaskMenuCmd =
  | { kind: 'action' }
  | { kind: 'startNew' }
  | { kind: 'moveStep'; stepId: string }
  | { kind: 'setStatus'; statusId: string }
  | { kind: 'stopRuns' }
  | { kind: 'openSession'; session: Session }
  | { kind: 'showSessions' }
  | { kind: 'automation'; scriptId: string }
  | { kind: 'plan'; day: number }
  | { kind: 'shelve'; shelf: 'later' | 'backlog' }
  | { kind: 'park'; parked: boolean }
  | { kind: 'makeTask' }
  | { kind: 'link'; url: string }
  | { kind: 'copy'; text: string }
  | { kind: 'review'; branchId: string }
  | { kind: 'changeBase'; act: RebaseAct }
  | { kind: 'archive' };

export interface TaskMenuItem {
  id: string;
  label: string;
  icon?: string;
  cmd: TaskMenuCmd;
  disabled?: boolean;
  hint?: string | readonly TextPart[];
  shortcut?: ShortcutId;
  checked?: boolean;
  danger?: boolean;
}

export type TaskMenuEntry =
  | { type: 'label'; label: string }
  | { type: 'separator' }
  | ({ type: 'item' } & TaskMenuItem)
  | { type: 'submenu'; id: string; label: string; items: TaskMenuItem[] };

export interface TaskMenuInput {
  card: CardModel;
  /** The task's sessions. */
  sessions: readonly Session[];
  /** Menu text of a running session. */
  sessionLabel: (s: Session) => string;
  plan: Plan;
  /** The status set, in order. */
  statuses: readonly TaskStatusDef[];
  cal: Calendar;
  archivePending: boolean;
  /** The task's Review code targets; none for a parked task. */
  choices: readonly ReviewChoice[];
  /** The task's Change base targets. */
  baseChoices: readonly ChangeBaseChoice[];
  /** The Run automation submenu items. */
  automations: readonly TaskMenuItem[];
  /** Name of the task link's code host, for "Open on …". */
  hostLabel: string;
}

const DONE = 'done';
const liveHint = (): string => t('space.ade.board.taskMenu.liveBlocked.tooltip');

function copyItems(c: CardModel, sessions: readonly Session[]): TaskMenuItem[] {
  const items: TaskMenuItem[] = [];
  if (c.title)
    items.push({
      id: 'ade-task-copy-title',
      label: t('common.field.title.label'),
      cmd: { kind: 'copy', text: c.title },
    });
  items.push({
    id: 'ade-task-copy-id',
    label: t('space.ade.board.taskMenu.taskId.label'),
    cmd: { kind: 'copy', text: c.task.id },
  });
  if (c.task.jira) {
    items.push({
      id: 'ade-task-copy-jira',
      label: t('space.ade.board.taskMenu.jiraLink.label'),
      cmd: { kind: 'copy', text: c.task.jira.url },
    });
  }
  const names = c.rows
    .filter((r) => r.branch.kind === 'mine' && r.branch.name)
    .map((r) => r.branch.name);
  if (names.length) {
    items.push({
      id: 'ade-task-copy-branches',
      label: t('space.ade.board.taskMenu.branchNames.label'),
      cmd: { kind: 'copy', text: names.join('\n') },
    });
  }
  const latest = sessions
    .filter((s) => s.agentSessionId)
    .sort((a, b) => b.lastActiveAt - a.lastActiveAt)[0];
  if (latest) {
    items.push({
      id: 'ade-task-copy-session',
      label: t('space.ade.board.taskMenu.latestSessionId.label'),
      cmd: { kind: 'copy', text: latest.agentSessionId },
    });
  }
  return items;
}

function stepEntries(c: CardModel, live: boolean): TaskMenuEntry[] {
  if (!c.blocks.length) return [];
  const current = c.progress.step?.id ?? c.task.entryStepId;
  const items: TaskMenuItem[] = [
    ...c.blocks.map((b) => ({
      id: `ade-task-step-${b.step.id}`,
      label: b.step.name,
      cmd: { kind: 'moveStep', stepId: b.step.id } as const,
      checked: b.step.id === current,
      disabled: live || (b.state === 'skipped' && b.step.id !== current),
      hint: live ? liveHint() : undefined,
    })),
    {
      id: 'ade-task-step-done',
      label: t('common.done.status'),
      cmd: { kind: 'moveStep', stepId: DONE },
      checked: current === DONE,
      disabled: live,
      hint: live ? liveHint() : undefined,
    },
  ];
  const out: TaskMenuEntry[] = [
    {
      type: 'submenu',
      id: 'ade-task-step',
      label: t('space.ade.board.taskMenu.moveToStep.label'),
      items,
    },
  ];
  const at = c.blocks.find((b) => b.step.id === current);
  if (at) {
    out.push({
      type: 'item',
      id: 'ade-task-skip',
      label: t('space.ade.board.taskMenu.skipStep.label', { step: at.step.name }),
      cmd: { kind: 'moveStep', stepId: nextStepId(c.blocks, current) },
      disabled: live,
      hint: live ? liveHint() : undefined,
    });
  }
  return out;
}

function statusEntry(c: CardModel, statuses: readonly TaskStatusDef[]): TaskMenuEntry {
  return {
    type: 'submenu',
    id: 'ade-task-status',
    label: t('common.field.status.label'),
    items: statuses.map((s) => ({
      id: `ade-task-status-${s.id}`,
      label: s.name,
      cmd: { kind: 'setStatus', statusId: s.id },
      checked: s.id === c.status.id,
    })),
  };
}

function reviewEntry(choices: readonly ReviewChoice[]): TaskMenuEntry[] {
  const item = (c: ReviewChoice, id: string, label: string): TaskMenuItem => ({
    id,
    label,
    cmd: { kind: 'review', branchId: c.branchId },
    disabled: c.disabled,
    hint: c.tip,
  });
  if (choices.length === 0) return [];
  if (choices.length === 1) {
    return [
      {
        type: 'item',
        ...item(choices[0], 'ade-task-review', t('space.ade.board.taskMenu.reviewCode.label')),
        shortcut: 'ade.reviewCode',
      },
    ];
  }
  return [
    {
      type: 'submenu',
      id: 'ade-task-review',
      label: t('space.ade.board.taskMenu.reviewCode.label'),
      items: choices.map((c) => item(c, `ade-task-review-${c.branchId}`, c.label)),
    },
  ];
}

function changeBaseEntry(choices: readonly ChangeBaseChoice[]): TaskMenuEntry[] {
  const item = (c: ChangeBaseChoice, id: string, label: string): TaskMenuItem => ({
    id,
    label,
    cmd: { kind: 'changeBase', act: c.act },
    disabled: c.disabled,
    hint: c.tip,
  });
  if (choices.length === 0) return [];
  if (choices.length === 1) {
    return [
      {
        type: 'item',
        ...item(choices[0], 'ade-task-change-base', t('space.ade.rebase.changeBase.label')),
      },
    ];
  }
  return [
    {
      type: 'submenu',
      id: 'ade-task-change-base',
      label: t('space.ade.rebase.changeBase.label'),
      items: choices.map((c) => item(c, `ade-task-change-base-${c.branchId}`, c.label)),
    },
  ];
}

function planEntry(i: TaskMenuInput): TaskMenuEntry {
  const day = i.card.entry.auto ? LATER : i.card.entry.day;
  const at = (id: string, label: string, to: number): TaskMenuItem => {
    // A shelved task has no day: only the Later shelf counts as "on Later", so Backlog tasks can move there.
    const here = to === LATER ? i.card.shelf === 'later' || i.card.entry.auto : day === to;
    return { id, label, cmd: { kind: 'plan', day: to }, checked: here, disabled: here };
  };
  return {
    type: 'submenu',
    id: 'ade-task-plan',
    label: t('space.common.plan.label'),
    items: [
      at('ade-task-plan-today', t('space.ade.board.taskMenu.moveToday.label'), 0),
      at(
        'ade-task-plan-next',
        t('space.ade.board.taskMenu.moveNextWorkDay.label'),
        nextWork(i.cal, 0),
      ),
      at('ade-task-plan-later', t('space.ade.board.taskMenu.moveLater.label'), LATER),
    ],
  };
}

export function taskMenuModel(i: TaskMenuInput): TaskMenuEntry[] {
  const c = i.card;
  const review = c.review;
  const out: TaskMenuEntry[] = [{ type: 'label', label: c.title }, ...reviewEntry(i.choices)];
  const live = c.task.runs.some((r) => r.state === 'running');
  const running = i.sessions.filter((s) => s.state === 'running' && s.mode === 'tui');

  if (review) {
    out.push(
      {
        type: 'item',
        id: 'ade-task-make-task',
        label: t('space.ade.board.taskMenu.makeTask.label'),
        cmd: { kind: 'makeTask' },
        hint: t('space.ade.board.taskMenu.makeTask.tooltip', {
          owner: c.task.owner || t('space.ade.taskPanel.someone.label'),
        }),
      },
      statusEntry(c, i.statuses),
      { type: 'separator' },
    );
  }
  if (!review) {
    if (c.action && c.action.kind !== 'archive') {
      out.push({
        type: 'item',
        id: 'ade-task-action',
        label: c.action.label.replace(/^▶ /, ''),
        cmd: { kind: 'action' },
        hint: c.action.tip,
      });
    }
    if (c.action?.kind === 'resume') {
      const step = c.progress.step?.name ?? '';
      out.push({
        type: 'item',
        id: 'ade-task-start-new',
        label: t('space.ade.board.taskMenu.startNew.label', { step }),
        cmd: { kind: 'startNew' },
        hint: t('space.ade.board.taskMenu.startNew.tooltip', { step }),
      });
    }
    out.push(statusEntry(c, i.statuses), ...stepEntries(c, live));
    if (c.task.runs.some((r) => r.state === 'running' || r.state === 'pending')) {
      out.push({
        type: 'item',
        id: 'ade-task-stop',
        label: t('space.ade.board.taskMenu.stopRuns.label'),
        cmd: { kind: 'stopRuns' },
      });
    }
    out.push({ type: 'separator' });
  }

  if (running.length) {
    out.push({
      type: 'submenu',
      id: 'ade-task-open-session',
      label: t('space.ade.board.taskMenu.openSession.label'),
      items: running.map((s) => ({
        id: `ade-task-open-session-${s.id}`,
        label: i.sessionLabel(s),
        cmd: { kind: 'openSession', session: s },
      })),
    });
  }
  out.push({
    type: 'item',
    id: 'ade-task-sessions',
    label: t('space.ade.board.taskMenu.showSessions.label'),
    cmd: { kind: 'showSessions' },
  });
  out.push({
    type: 'submenu',
    id: 'ade-task-automation',
    label: t('space.ade.board.taskMenu.runAutomation.label'),
    items: [...i.automations],
  });

  out.push(planEntry(i));
  if (!review) {
    out.push({
      type: 'item',
      id: 'ade-task-park',
      label: t(
        c.parked
          ? 'space.ade.board.taskMenu.markMerging.label'
          : 'space.ade.board.taskMenu.markNotMerging.label',
      ),
      cmd: { kind: 'park', parked: !c.parked },
    });
    out.push(...changeBaseEntry(i.baseChoices));
  }

  const links: TaskMenuItem[] = [];
  if (c.task.jira) {
    links.push({
      id: 'ade-task-jira',
      label: t('space.ade.board.taskMenu.openJira.label'),
      cmd: { kind: 'link', url: c.task.jira.url },
    });
  }
  if (c.task.hostUrl) {
    links.push({
      id: 'ade-task-host',
      label: t('space.ade.board.taskMenu.openHost.label', { label: i.hostLabel }),
      cmd: { kind: 'link', url: c.task.hostUrl },
    });
  }
  if (links.length)
    out.push({ type: 'separator' }, ...links.map((l) => ({ type: 'item' as const, ...l })));

  const copy = copyItems(c, i.sessions);
  out.push({
    type: 'submenu',
    id: 'ade-task-copy',
    label: t('common.action.copy.label'),
    items: copy,
  });
  out.push({ type: 'separator' });
  if (c.shelf === 'backlog') {
    out.push({
      type: 'item',
      id: 'ade-task-later',
      label: t('space.ade.board.taskMenu.moveLater.label'),
      cmd: { kind: 'shelve', shelf: 'later' },
    });
  } else if (!review) {
    out.push({
      type: 'item',
      id: 'ade-task-backlog',
      label: t('space.ade.board.taskMenu.moveBacklog.label'),
      cmd: { kind: 'shelve', shelf: 'backlog' },
    });
  }
  out.push({
    type: 'item',
    id: 'ade-task-archive',
    label: t('space.common.archive.label'),
    cmd: { kind: 'archive' },
    danger: true,
    disabled: i.archivePending,
  });
  return out;
}

// ---- Day context menu
export interface DayMenu {
  label: string;
  patch: SettingsPatch['ade'];
  /** Marking a worked weekday off, with tasks starting that day: offer to move them. */
  confirm: boolean;
}

/** Day context menu, mockup v2 lines 1315-1332. `null` on Later and past days; off > weekend > weekday. */
export function dayMenuFor(
  cal: Calendar,
  settings: Settings['ade'],
  today: string,
  day: number,
  startCount: number,
): DayMenu | null {
  if (day === LATER || day < 0) return null;
  const iso = offsetToIso(today, day);
  if (cal.offDays.has(day)) {
    return {
      label: t('space.ade.board.dayMenu.workingDay.label'),
      patch: { offDays: settings.offDays.filter((d) => d !== iso) },
      confirm: false,
    };
  }
  if (isWeekend(cal, day)) {
    return cal.workWeekend.has(day)
      ? {
          label: t('space.ade.board.dayMenu.weekend.label'),
          patch: { workWeekendDays: settings.workWeekendDays.filter((d) => d !== iso) },
          confirm: false,
        }
      : {
          label: t('space.ade.board.dayMenu.workThisDay.label'),
          patch: { workWeekendDays: [...settings.workWeekendDays, iso] },
          confirm: false,
        };
  }
  return {
    label: t('space.ade.board.dayMenu.dayOff.label'),
    patch: { offDays: [...settings.offDays.filter((d) => d >= today), iso] },
    confirm: startCount > 0,
  };
}

// ---- Branch row fix menu
/** The branch row's right-click menu (mockup `branchMenu`): what can be fixed on a created mine
 *  branch, one item per fix. */

export type FixItem =
  | { id: string; kind: 'merge'; label: string; target: string }
  | { id: string; kind: 'act'; label: string; act: RebaseAct; hint: readonly TextPart[] }
  | { id: string; kind: 'forcePush'; label: string };

export interface FixInput {
  branchId: string;
  graph: BranchGraph;
  /** The repo's integration branches. */
  targets: readonly string[];
  /** `TimelineView.after`: earlier-merging branch this one shares files with. */
  after: ReadonlyMap<string, { id: string; file: string }>;
  unpushed: Readonly<Record<string, boolean>>;
  runs: readonly Run[];
  /** Short name of the repo's main branch, `''` when unresolved. */
  mainName: string;
}

const plain = (parts: readonly TextPart[]): string =>
  parts.map((p) => (typeof p === 'string' ? p : p.value)).join('');

/** `null` for a draft or a branch that is not mine: those get no menu. */
export function fixItems(i: FixInput): FixItem[] | null {
  const b = i.graph.byBranch.get(i.branchId);
  if (!b || b.name === '' || b.kind !== 'mine') return null;
  const out: FixItem[] = [];
  for (const target of i.targets) {
    const status = b.integration.find((x) => x.target === target)?.status ?? 'not merged';
    if (status === 'merged') continue;
    out.push({
      id: `ade-fix-merge-${target}`,
      kind: 'merge',
      label: t(
        status === 'stale'
          ? 'space.ade.board.fixMenu.reMerge.label'
          : 'space.ade.board.fixMenu.merge.label',
        { target },
      ),
      target,
    });
  }
  const acts = rebaseActs({
    branch: b,
    graph: i.graph,
    after: i.after,
    runs: i.runs,
    mainName: i.mainName,
  });
  for (const act of acts) {
    out.push({ id: `ade-fix-${act.id}`, kind: 'act', label: plain(act.label), act, hint: act.tip });
  }
  if (i.unpushed[b.id])
    out.push({
      id: 'ade-fix-force-push',
      kind: 'forcePush',
      label: t('common.forcePush.label'),
    });
  return out;
}
