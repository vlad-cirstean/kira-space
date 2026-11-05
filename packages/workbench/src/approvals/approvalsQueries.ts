import type { ApprovalAnswerArgs, ApprovalItem, ApprovalResult } from '@shared/domain/approvals';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { errorText } from '@workbench/bridge/codedError';
import { useApprovalsStore } from '@workbench/state/approvals';
import { queryClient } from '@workbench/state/queryClient';
import { type InjectionKey, inject, onScopeDispose } from 'vue';

// P520: client side of the Go approvals queue (internal/approvals). Items are the whole view; each
// kind's owner decides what an answer does.

/** The queue's call and push one app's bridge provides (createCoreControl.ts). */
export interface ApprovalsControl {
  approvalsList(): Promise<ApprovalItem[]>;
  approvalsAnswer(args: ApprovalAnswerArgs): Promise<ApprovalResult[]>;
  onApprovalsChanged(cb: (items: ApprovalItem[]) => void): () => void;
}

export const approvalsControlKey: InjectionKey<ApprovalsControl> = Symbol('approvalsControl');

export function useApprovalsControl(): ApprovalsControl {
  const control = inject(approvalsControlKey);
  if (!control) throw new Error('approvalsControlKey is not provided');
  return control;
}

const APPROVALS_KEY = ['approvals'];

/** Every pending item, oldest first. A push replaces the cache. */
export function useApprovals() {
  const control = useApprovalsControl();
  const query = useQuery(
    { queryKey: APPROVALS_KEY, queryFn: () => control.approvalsList() },
    queryClient,
  );
  onScopeDispose(
    control.onApprovalsChanged((items) => queryClient.setQueryData(APPROVALS_KEY, items)),
  );
  return query;
}

/** Answers ids with one decision. The queue's push removes the settled items. */
export function useAnswerApprovals() {
  const control = useApprovalsControl();
  return useMutation(
    {
      mutationKey: ['approvals', 'answer'],
      mutationFn: (args: ApprovalAnswerArgs) => control.approvalsAnswer(args),
    },
    queryClient,
  );
}

/** Answers with the options each body collected; a refused item stays listed with its error. */
export function useApprovalActions() {
  const mutation = useAnswerApprovals();
  const store = useApprovalsStore();

  async function answer(ids: string[], approve: boolean): Promise<void> {
    if (ids.length === 0) return;
    store.begin(ids);
    const options = Object.fromEntries(
      ids.filter((id) => id in store.options).map((id) => [id, store.options[id]]),
    );
    try {
      const results = await mutation.mutateAsync({
        ids,
        approve,
        options: Object.keys(options).length > 0 ? options : undefined,
      });
      for (const r of results) if (r.error) store.fail(r.id, r.error);
    } catch (e) {
      for (const id of ids) store.fail(id, errorText(e));
    }
  }

  return { answer };
}
