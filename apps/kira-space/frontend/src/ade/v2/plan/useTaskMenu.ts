import { useCopy } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../../bridge/control';
import { useRunAutomation } from '../automation/useRunAutomation';
import { changeBaseChoices } from '../board/changeBase';
import { movePlanArgs } from '../board/dropPlan';
import {
  type TaskMenuCmd,
  type TaskMenuEntry,
  type TaskMenuItem,
  taskMenuModel,
} from '../board/menu';
import { taskPatch } from '../board/panelFacts';
import { reviewChoices } from '../board/reviewCode';
import { shelfIds } from '../board/shelf';
import { useHostLabels } from '../hostLabels';
import { useSetPlan, useShelveTask, useStopRun, useUpdateTask } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import { useTaskAction } from '../run/useTaskAction';
import { useSessionViews } from '../sessions/useSessionViews';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import { type CardModel, usePlanModel } from './usePlanModel';

/** The card box is not focusable; its header is. */
function focusable(el: Element): HTMLElement | null {
  if (!(el instanceof HTMLElement)) return null;
  return el.querySelector<HTMLElement>('[data-testid="ade-card-head"]') ?? el;
}

/** Opens the task context menu for `card`: on a right-click, or under an anchor element. */
export function useTaskMenu(card: MaybeRefOrGetter<CardModel>) {
  const ui = useAdeBoardUiStore();
  const dialogs = useAdeDialogsStore();
  const contextMenu = useContextMenuStore();
  const { model, today, sessions } = usePlanModel();
  const { view } = useSessionViews();
  const { perform } = useTaskAction(card);
  const stage = useAdeStageDialogStore();
  const setPlan = useSetPlan();
  const shelve = useShelveTask();
  const stopRun = useStopRun();
  const update = useUpdateTask();
  const { copy } = useCopy();
  const reviewCode = useReviewCode();
  const automation = useRunAutomation();
  const hostLabels = useHostLabels();
  let anchor: HTMLElement | null = null;

  function fail(taskId: string, err: unknown): void {
    ui.select(taskId);
    ui.actionError[taskId] = errorText(err);
  }

  async function run(cmd: TaskMenuCmd): Promise<void> {
    const c = toValue(card);
    const taskId = c.task.id;
    delete ui.actionError[taskId];
    try {
      switch (cmd.kind) {
        case 'action':
          return await perform();
        case 'moveStep':
          return stage.open({ taskId, toStepId: cmd.stepId });
        case 'setStatus':
          await update.mutateAsync({ taskId, patch: taskPatch({ statusId: cmd.statusId }) });
          return;
        case 'stopRuns':
          await Promise.all(
            c.task.runs
              .filter((r) => r.state === 'running' || r.state === 'pending')
              .map((r) => stopRun.mutateAsync({ runId: r.id })),
          );
          return;
        case 'openSession':
          return ui.openSession({
            taskId,
            branchId: cmd.session.branchId,
            sessionId: cmd.session.id,
          });
        case 'showSessions':
          return ui.openSessions(taskId);
        case 'automation':
          return automation.start(cmd.scriptId, taskId);
        case 'plan': {
          const m = model.value;
          if (m)
            await setPlan.mutateAsync(
              movePlanArgs(m.board.plan, today.value, taskId, null, cmd.day),
            );
          return;
        }
        case 'shelve': {
          const m = model.value;
          if (m)
            await shelve.mutateAsync({
              taskId,
              shelf: cmd.shelf,
              beforeId: shelfIds(m.board, cmd.shelf)[0] ?? '',
            });
          return;
        }
        case 'park':
          await update.mutateAsync({
            taskId,
            patch: taskPatch({ kind: cmd.parked ? 'parked' : 'task' }),
          });
          return;
        case 'makeTask':
          await update.mutateAsync({ taskId, patch: taskPatch({ kind: 'task' }) });
          return;
        case 'link':
          return await control.openExternal(cmd.url);
        case 'copy':
          await copy(cmd.text);
          return;
        case 'review':
          return reviewCode.open(taskId, cmd.branchId, anchor);
        case 'archive':
          return dialogs.archive(taskId);
        case 'changeBase':
          return dialogs.act(cmd.act);
      }
    } catch (err) {
      fail(taskId, err);
    }
  }

  function toItem(e: TaskMenuItem): MenuItem {
    return {
      type: 'item',
      id: e.id,
      label: e.label,
      icon: e.icon,
      disabled: e.disabled,
      hint: e.hint,
      shortcut: e.shortcut,
      checked: e.checked,
      danger: e.danger,
      run: () => run(e.cmd),
    };
  }
  function toMenu(e: TaskMenuEntry): MenuItem {
    if (e.type === 'item') return toItem(e);
    if (e.type === 'submenu')
      return { type: 'submenu', id: e.id, label: e.label, items: e.items.map(toItem) };
    return e;
  }

  function items(): MenuItem[] | null {
    const m = model.value;
    const c = toValue(card);
    if (!m) return null;
    return taskMenuModel({
      card: c,
      sessions: sessions.value.filter((s) => s.taskId === c.task.id),
      sessionLabel: (s) => view(s).tabName,
      plan: m.board.plan,
      statuses: m.board.statuses,
      cal: m.cal,
      archivePending: dialogs.pending.has(`archive:${c.task.id}`),
      automations: automation.items(),
      hostLabel:
        hostLabels.infoOf(c.task.hostLink?.provider ?? '')?.label ?? hostLabels.labels.value,
      choices: reviewChoices(c, m.view.graph),
      baseChoices: changeBaseChoices(c, {
        graph: m.view.graph,
        after: m.view.after,
        board: m.board,
      }),
    }).map(toMenu);
  }

  function open(ev: MouseEvent): void {
    const list = items();
    if (!list) return;
    ev.preventDefault();
    ev.stopPropagation();
    anchor = ev.currentTarget instanceof Element ? focusable(ev.currentTarget) : null;
    ui.select(toValue(card).task.id);
    contextMenu.openContextMenu(ev, list);
  }

  /** Under `el`, for the keyboard and the panel's More button. */
  function openAt(el: Element): void {
    const list = items();
    if (!list) return;
    anchor = focusable(el);
    ui.select(toValue(card).task.id);
    const r = el.getBoundingClientRect();
    contextMenu.openContextMenuAt(r.left, r.bottom, list);
  }

  return { open, openAt };
}
