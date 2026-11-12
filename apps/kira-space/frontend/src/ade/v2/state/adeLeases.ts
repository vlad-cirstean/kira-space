import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  useForceLeaseRelease,
  useLeases,
  useRequestLeaseRelease,
  useSaveLeaseQueues,
} from '../leases/queries';
import type { LeaseQueue } from '../wire';

// Lease panel state: the new-queue draft and the actions on queues and holders. The queues
// themselves are server state (`useLeases`).
export const useAdeLeasesStore = defineStore('adeLeases', () => {
  const leases = useLeases();
  const save = useSaveLeaseQueues();
  const request = useRequestLeaseRelease();
  const force = useForceLeaseRelease();
  const draft = ref('');

  const names = (): string[] => (leases.data.value?.queues ?? []).map((q) => q.name);
  const fail = (err: unknown): void => void toast.error(errorText(err));

  function canAdd(): boolean {
    const name = draft.value.trim();
    return name !== '' && !names().includes(name);
  }

  async function addQueue(): Promise<void> {
    if (!canAdd()) return;
    try {
      await save.mutateAsync({ names: [...names(), draft.value.trim()] });
      draft.value = '';
    } catch (err) {
      fail(err);
    }
  }

  async function removeQueue(name: string): Promise<void> {
    try {
      await save.mutateAsync({ names: names().filter((n) => n !== name) });
    } catch (err) {
      fail(err);
    }
  }

  async function requestRelease(queue: string): Promise<void> {
    try {
      await request.mutateAsync({ queue });
    } catch (err) {
      fail(err);
    }
  }

  async function forceRelease(q: LeaseQueue): Promise<void> {
    const ok = await useConfirmDialogStore().confirm({
      title: t('space.ade.leases.forceConfirm.title', { queue: q.name }),
      message: t('space.ade.leases.forceConfirm.body', { task: q.holder?.taskTitle ?? '' }),
      confirmLabel: t('space.ade.leases.forceRelease.label'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await force.mutateAsync({ queue: q.name });
    } catch (err) {
      fail(err);
    }
  }

  return { draft, canAdd, addQueue, removeQueue, requestRelease, forceRelease };
});
