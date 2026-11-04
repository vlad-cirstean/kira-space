import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { toCodedError } from '@workbench/bridge/codedError';
import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useApproveBranchRequest, useBranchRequest, useDenyBranchRequest } from '../queries';

// The branch request dialog: which request it is open for and which items are checked. One concern:
// answering an agent's request. The Task tab row and the routed `branch-request` prompt open it here.
export const useAdeBranchRequestStore = defineStore('adeBranchRequest', () => {
  const { model } = usePlanModel();
  const approveM = useApproveBranchRequest();
  const denyM = useDenyBranchRequest();

  const id = ref('');
  /** Opened by the routed prompt rather than by the Task tab. */
  const routed = ref(false);
  /** Unchecked item ids; every item starts checked. */
  const unchecked = ref<string[]>([]);
  const query = useBranchRequest(id);

  const request = computed(() => query.data.value ?? null);
  const isOpen = computed(() => id.value !== '' && request.value !== null);
  const selected = computed(
    () => request.value?.items.map((i) => i.id).filter((c) => !unchecked.value.includes(c)) ?? [],
  );

  function open(args: { id: string; routed?: boolean }): void {
    id.value = args.id;
    routed.value = args.routed ?? false;
    unchecked.value = [];
  }

  function close(): void {
    id.value = '';
  }

  function toggle(itemId: string): void {
    unchecked.value = unchecked.value.includes(itemId)
      ? unchecked.value.filter((c) => c !== itemId)
      : [...unchecked.value, itemId];
  }

  // Answered elsewhere (a timeout, another window, the agent stopped): nothing left to decide.
  watch(
    () => query.error.value,
    (err) => {
      if (err && toCodedError(err).code === 'E_NOT_FOUND') close();
    },
  );
  // A Task tab opening follows the board; a routed one follows its prompt, which can precede the board.
  watch(
    () => {
      const r = request.value;
      return r && !routed.value ? model.value?.cardFor(r.taskId)?.task.branchRequestId : undefined;
    },
    (current) => {
      if (current === '') close();
    },
  );

  const approval = useAsyncAction(async () => {
    await approveM.mutateAsync({ id: id.value, itemIds: selected.value });
    close();
  });
  const denial = useAsyncAction(async () => {
    await denyM.mutateAsync({ id: id.value });
    close();
  });

  const error = computed(() => approval.error.value ?? denial.error.value ?? '');
  const pending = computed(() => approval.pending.value || denial.pending.value);

  return {
    id,
    routed,
    request,
    selected,
    isOpen,
    error,
    pending,
    open,
    close,
    toggle,
    approve: approval.run,
    deny: denial.run,
  };
});
