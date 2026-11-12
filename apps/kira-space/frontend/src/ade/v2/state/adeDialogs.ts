import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';
import type { RebaseAct } from '../board/rebaseActions';
import {
  changeBaseSpec,
  type DialogSpec,
  mergeSpec,
  rebaseSpec,
  sessionSpec,
  startSpec,
} from '../dialog/compose';
import { deleteAnyway, type FlowDeps, requestArchive, sendDialog } from '../dialog/flow';
import { adeTurns } from '../dialog/turnWatch';
import { useDialogCtx } from '../dialog/useDialogCtx';
import { useLaunchOpener } from '../dialog/useLaunchOpener';
import {
  useAbortRebase,
  useArchiveRisk,
  useArchiveTask,
  useLaunchStep,
  useRebase,
  useRecordMerge,
  useSend,
  useSetBranchBase,
  useStartBranch,
} from '../queries';
import type { BaseChoice } from '../wire';
import { useAdeBoardUiStore } from './adeBoardUi';
import { useAdeOpsStore } from './adeOps';
import { useRepoGate } from './useRepoGate';
import { useSetupWait } from './useSetupWait';

// The Claude dialog: which one is open, its edits and the work in flight behind it. One concern:
// the dialog flow. Every opener (action cell, branch header, fix menu, Details, Needs you) goes
// through here, so the delivery rules live once.
export const useAdeDialogsStore = defineStore('adeDialogs', () => {
  const ui = useAdeBoardUiStore();
  const ops = useAdeOpsStore();
  const ctx = useDialogCtx();
  const launchOpener = useLaunchOpener();
  const confirmDialog = useConfirmDialogStore();
  const { retryAfterRepo } = useRepoGate();

  const spec = ref<DialogSpec | null>(null);
  /** `null` = the unedited template. */
  const msg = ref<string | null>(null);
  const push = ref(false);
  const override = ref(false);
  const autostash = ref(false);
  const error = ref('');
  /** The branch an Abort rebase confirmation is open for. */
  const abortTarget = ref<{ branchId: string; name: string } | null>(null);
  /** In-flight work: `rebase:<branch>`, `merge:<branch>:<target>`, `archive:<task>`. */
  const pending = reactive(new Set<string>());

  const sendM = useSend();
  const startM = useStartBranch();
  const stepM = useLaunchStep();
  const rebaseM = useRebase();
  const baseM = useSetBranchBase();
  const abortM = useAbortRebase();
  const mergeM = useRecordMerge();
  const riskM = useArchiveRisk();
  const archiveM = useArchiveTask();

  // A launch refused behind a running prepare script retries by itself until the worktree is ready.
  const setupWait = useSetupWait(() => send());
  const waitingSetup = setupWait.waiting;

  function open(next: DialogSpec): void {
    setupWait.stop();
    spec.value = next;
    msg.value = null;
    push.value = false;
    override.value = false;
    autostash.value = false;
    error.value = '';
  }

  function close(): void {
    setupWait.stop();
    spec.value = null;
  }

  function deps(): FlowDeps | null {
    const c = ctx.value;
    if (!c) return null;
    return {
      ctx: c,
      launch: {
        send: (a) => sendM.mutateAsync(a),
        startBranch: (a) => startM.mutateAsync(a),
        launchStep: (a) => retryAfterRepo(a.taskId, () => stepM.mutateAsync(a)),
        ...launchOpener.deps,
      },
      turns: { watch: adeTurns.watch },
      rebase: (a) => {
        const b = c.graph.byBranch.get(a.branchId);
        void ops.run(
          {
            id: `rebase:${a.branchId}`,
            kind: 'rebase',
            taskId: b?.taskId || undefined,
            branchId: a.branchId,
            name: b?.name ?? '',
          },
          () => rebaseM.mutateAsync(a),
        );
      },
      setBranchBase: (a) => baseM.mutateAsync(a),
      recordMerge: (a) => mergeM.mutateAsync(a),
      // Runs in the background: the dialog is gone at once, the op's chip and toast report it.
      archiveTask: (a) => {
        ops.run(
          {
            id: `archive:${a.taskId}`,
            kind: 'archive',
            taskId: a.taskId,
            name: c.taskTitle(a.taskId),
          },
          () => archiveM.mutateAsync(a),
        );
        return Promise.resolve();
      },
      archiveRisk: (a) => riskM.mutateAsync(a),
      confirmArchive: (taskId) =>
        confirmDialog.confirm({
          tone: 'danger',
          title: t('space.ade.archiveConfirm.title'),
          message: t('space.ade.archiveConfirm.body', { title: c.taskTitle(taskId) }),
          confirmLabel: t('space.common.archive.label'),
        }),
      pending: { add: (k) => pending.add(k), remove: (k) => pending.delete(k) },
      setError: (m) => {
        setupWait.stop();
        error.value = m;
      },
      waitForSetup: () => {
        error.value = '';
        setupWait.start();
      },
      setActionError: (taskId, m) => {
        ui.actionError[taskId] = m;
      },
      closeDialog: close,
      openDialog: open,
      opened: (e) => ui.openSession(e),
    };
  }

  async function send(): Promise<void> {
    const d = deps();
    if (d && spec.value) {
      error.value = '';
      await sendDialog(d, spec.value, {
        msg: msg.value,
        push: push.value,
        override: override.value,
        autostash: autostash.value,
        preview: null,
      });
    }
  }

  async function discardAnyway(): Promise<void> {
    const d = deps();
    if (d && spec.value) await deleteAnyway(d, spec.value);
  }

  async function archive(taskId: string): Promise<void> {
    const d = deps();
    if (!d) return;
    delete ui.actionError[taskId];
    await requestArchive(d, taskId);
  }

  /** Runs a rebase act: the rebase kinds open the dialog, Abort rebase asks first. */
  function act(a: RebaseAct): void {
    const c = ctx.value;
    if (a.kind === 'abortRebase') {
      abortTarget.value = {
        branchId: a.branchId,
        name: c?.graph.byBranch.get(a.branchId)?.name ?? '',
      };
    } else if (c) open(a.kind === 'changeBase' ? changeBaseSpec(c, a) : rebaseSpec(c, a));
  }

  /** The Abort rebase confirmation's yes: an error message keeps it open, `null` closes it. */
  async function confirmAbort(): Promise<string | null> {
    const t = abortTarget.value;
    if (!t) return null;
    try {
      await abortM.mutateAsync({ branchId: t.branchId });
      return null;
    } catch (err) {
      return errorText(err);
    }
  }

  /** The base picker's choice in a Change base dialog; the edited prompt no longer fits it. */
  function pickBase(choice: BaseChoice): void {
    if (!spec.value) return;
    spec.value = { ...spec.value, onto: choice };
    msg.value = null;
  }

  function merge(branchId: string, target: string): void {
    const c = ctx.value;
    if (c) open(mergeSpec(c, branchId, target));
  }

  function session(taskId: string): void {
    const c = ctx.value;
    if (c) open(sessionSpec(c, taskId));
  }

  function start(branchId: string): void {
    open(startSpec(branchId));
  }

  function pick(branchId: string, choice: string): void {
    const s = spec.value;
    if (!s) return;
    spec.value = {
      ...s,
      targets: s.targets.map((t) => (t.branchId === branchId ? { ...t, choice } : t)),
    };
  }

  return {
    spec,
    msg,
    push,
    override,
    autostash,
    abortTarget,
    error,
    waitingSetup,
    pending,
    ctx,
    close,
    send,
    discardAnyway,
    archive,
    act,
    confirmAbort,
    pickBase,
    merge,
    session,
    start,
    pick,
  };
});
