import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';
import { useLaunchOpener } from '../dialog/useLaunchOpener';
import { useRestoreTask, useTakeOver } from '../queries';
import { useSessionViews } from '../sessions/useSessionViews';
import { useAdeBoardUiStore } from './adeBoardUi';
import { isPreparing, useSetupWait } from './useSetupWait';

export interface TakeOverConfirm {
  sessionId: string;
  text: string;
}

// Take over: continue a session in an interactive Claude Code terminal. One store so every surface
// (Sessions tab, step run lines, task action, Needs you, `!`) shares the confirm for a run that is
// still going and the launch behind it.
export const useAdeTakeOverStore = defineStore('adeTakeOver', () => {
  const ui = useAdeBoardUiStore();
  const launchOpener = useLaunchOpener();
  const takeOverM = useTakeOver();
  const restoreM = useRestoreTask();
  const { sessions, view } = useSessionViews();

  /** The confirm for a session whose run is still running; `null` when none is open. */
  const confirm = ref<TakeOverConfirm | null>(null);
  /** Session ids with a take over in flight. */
  const pending = reactive(new Set<string>());

  /** The session whose take over waits for its worktree's prepare script, if any. */
  const waitingSession = ref<string | null>(null);
  const setupWait = useSetupWait(async () => {
    const id = waitingSession.value;
    if (!id) return setupWait.stop();
    const err = await launch(id);
    if (waitingSession.value !== id) return;
    if (err !== null) {
      const taskId = sessions.value.find((x) => x.id === id)?.taskId ?? '';
      if (taskId) ui.actionError[taskId] = err;
    }
    if (err !== null || !setupWait.waiting.value) cancelWait();
  });

  function cancelWait(): void {
    waitingSession.value = null;
    setupWait.stop();
  }

  /** Launches the take over. Resolves with an error message, `null` on success or while it waits. */
  async function launch(sessionId: string): Promise<string | null> {
    if (pending.has(sessionId)) return null;
    const s = sessions.value.find((x) => x.id === sessionId);
    pending.add(sessionId);
    try {
      const l = await takeOverM.mutateAsync({ sessionId, stopIfRunning: true });
      await launchOpener.open(l);
      ui.openSession({
        taskId: s?.taskId ?? '',
        branchId: s?.branchId ?? '',
        sessionId: l.sessionId,
      });
      if (waitingSession.value === sessionId) cancelWait();
      return null;
    } catch (err) {
      if (isPreparing(err)) {
        waitingSession.value = sessionId;
        setupWait.start();
        return null;
      }
      return errorText(err);
    } finally {
      pending.delete(sessionId);
    }
  }

  /** Takes over `sessionId`, asking first when its run is still running. */
  async function request(sessionId: string): Promise<void> {
    const s = sessions.value.find((x) => x.id === sessionId);
    const v = s ? view(s) : null;
    if (v?.headless && v.run?.state === 'running') {
      confirm.value = {
        sessionId,
        text: v.branch
          ? t('space.ade.takeOverConfirm.body', { step: v.step, branch: v.branch })
          : t('space.ade.takeOverTaskConfirm.body', { step: v.step }),
      };
      return;
    }
    const taskId = s?.taskId ?? '';
    if (taskId) delete ui.actionError[taskId];
    const err = await launch(sessionId);
    if (err && taskId) ui.actionError[taskId] = err;
  }

  /** Resume in an archived task: restores the task to Later, then takes the session over. */
  async function resumeArchived(taskId: string, sessionId: string): Promise<void> {
    if (pending.has(sessionId)) return;
    delete ui.actionError[taskId];
    pending.add(sessionId);
    try {
      await restoreM.mutateAsync({ taskId });
    } catch (err) {
      ui.actionError[taskId] = errorText(err);
      return;
    } finally {
      pending.delete(sessionId);
    }
    const err = await launch(sessionId);
    if (err) ui.actionError[taskId] = err;
  }

  /** The confirm's `Take over`: an error keeps the dialog open. */
  async function confirmYes(): Promise<string | null> {
    const c = confirm.value;
    return c ? launch(c.sessionId) : null;
  }

  function cancel(): void {
    confirm.value = null;
    cancelWait();
  }

  return { confirm, pending, waitingSession, request, resumeArchived, confirmYes, cancel };
});
