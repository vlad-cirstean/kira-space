import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { useAdeBoardUiStore } from './adeBoardUi';

export type AdeOpKind = 'removeBranch' | 'removeRepo' | 'archive';

export interface AdeOp {
  /** One op per id at a time: `<kind>:<branch or task>`. */
  id: string;
  kind: AdeOpKind;
  taskId: string;
  branchId?: string;
  /** What the op acts on, named in the toast. */
  name: string;
}

export function opLoadingText(kind: AdeOpKind): string {
  switch (kind) {
    case 'removeBranch':
      return t('space.ade.ops.removeBranch.loading');
    case 'removeRepo':
      return t('space.ade.ops.removeRepo.loading');
    case 'archive':
      return t('space.ade.ops.archive.loading');
  }
}

function opDoneText(op: AdeOp): string {
  switch (op.kind) {
    case 'removeBranch':
      return t('space.ade.ops.removeBranch.status', { name: op.name });
    case 'removeRepo':
      return t('space.ade.ops.removeRepo.status', { name: op.name });
    case 'archive':
      return t('space.ade.ops.archive.status', { name: op.name });
  }
}

// Destructive ADE ops (delete branch, remove repo, close task) run in the background once
// confirmed: the dialog is gone at once, rows show the op as a chip and a toast reports the result.
// One concern: in-flight ops.
export const useAdeOpsStore = defineStore('adeOps', () => {
  const ui = useAdeBoardUiStore();
  const ops = reactive(new Map<string, AdeOp>());

  function forTask(taskId: string): AdeOp[] {
    return [...ops.values()].filter((o) => o.taskId === taskId);
  }

  function forBranch(taskId: string, branchId: string): AdeOp[] {
    return forTask(taskId).filter((o) => o.branchId === branchId);
  }

  /** Starts `fn` and returns at once; a second run of a live id is ignored. */
  function run(op: AdeOp, fn: () => Promise<unknown>): void {
    if (ops.has(op.id)) return;
    ops.set(op.id, op);
    delete ui.actionError[op.taskId];
    toast.loading(opLoadingText(op.kind), { id: op.id });
    fn().then(
      () => {
        ops.delete(op.id);
        toast.success(opDoneText(op), { id: op.id });
      },
      (err: unknown) => {
        ops.delete(op.id);
        const message = errorText(err);
        toast.error(message, { id: op.id });
        ui.actionError[op.taskId] = message;
      },
    );
  }

  return { ops, forTask, forBranch, run };
});
