import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import { newIntentKey, useAdeWrites } from './useAdeWrites';
import type { PlanCard } from './useAgentsModel';

export type TaskIntent = 'back' | 'next' | 'start';

const message = (err: unknown): string => errorText(err);
const currentStepId = (c: PlanCard): string =>
  c.progress.finished ? 'done' : (c.progress.step?.id ?? c.task.entryStepId);

/** The Back, Next and Start buttons of one plan card: the confirm text for each, and the write. */
export function useTaskActions(card: () => PlanCard) {
  const writes = useAdeWrites();
  const intent = ref<TaskIntent | null>(null);
  const error = ref('');
  /** Handover notes for a Back or Next move; empty sends none. */
  const notes = ref('');
  let key = newIntentKey();

  const stepName = computed(() => card().progress.step?.name ?? '');
  /** Start maps to the same two calls the desktop's action makes; other actions stay desktop only. */
  const startKind = computed<'run' | 'launch' | null>(() => {
    const kind = card().action?.kind;
    if (kind === 'run') return 'run';
    // Resume is desktop only; the phone's Start still opens a new session.
    return kind === 'session' || kind === 'resume' ? 'launch' : null;
  });
  const busy = computed(
    () =>
      writes.setTaskStep.isPending.value ||
      writes.startRun.isPending.value ||
      writes.launchStep.isPending.value,
  );

  const dialog = computed(() => {
    const c = card();
    switch (intent.value) {
      case 'back':
        return {
          title: t('mobile.plan.moveBackConfirm.title'),
          text: t('mobile.plan.moveBackConfirm.body', {
            title: c.title,
            step: c.moves.prev?.name ?? '',
          }),
          confirmLabel: t('mobile.plan.moveBackConfirm.confirm'),
        };
      case 'next': {
        const done = c.moves.next?.name === 'Done';
        return {
          title: t(
            done ? 'mobile.plan.markDoneConfirm.title' : 'mobile.plan.moveForwardConfirm.title',
          ),
          text: t('mobile.plan.moveForwardConfirm.body', {
            title: c.title,
            step: c.moves.next?.name ?? '',
          }),
          confirmLabel: t(
            done ? 'mobile.plan.markDoneConfirm.confirm' : 'mobile.plan.moveForwardConfirm.confirm',
          ),
        };
      }
      case 'start':
        return {
          title: stepName.value
            ? t('mobile.plan.startConfirm.title', { step: stepName.value })
            : t('mobile.plan.startThisStepConfirm.title'),
          text: startText(c.title),
          confirmLabel: t('common.action.start.label'),
        };
      default:
        return { title: '', text: '', confirmLabel: '' };
    }
  });

  function startText(title: string): string {
    if (startKind.value === 'run') return t('mobile.plan.startRunConfirm.body', { title });
    return stepName.value
      ? t('mobile.plan.startSessionConfirm.body', { step: stepName.value })
      : t('mobile.plan.startThisStepSessionConfirm.body');
  }

  function ask(next: TaskIntent): void {
    error.value = '';
    notes.value = '';
    key = newIntentKey();
    intent.value = next;
  }

  function cancel(): void {
    intent.value = null;
  }

  async function confirm(): Promise<void> {
    const c = card();
    const from = currentStepId(c);
    error.value = '';
    try {
      if (intent.value === 'back' || intent.value === 'next') {
        const target = intent.value === 'back' ? c.moves.prev : c.moves.next;
        if (!target) return;
        await writes.setTaskStep.mutateAsync({
          taskId: c.task.id,
          fromStepId: from,
          stepId: target.id,
          key,
          ...(notes.value.trim() ? { notes: notes.value.trim() } : {}),
        });
      } else if (intent.value === 'start' && startKind.value === 'run') {
        await writes.startRun.mutateAsync({ taskId: c.task.id, fromStepId: from, key });
      } else if (intent.value === 'start' && startKind.value === 'launch') {
        await writes.launchStep.mutateAsync({ taskId: c.task.id, fromStepId: from, key });
      }
      intent.value = null;
    } catch (err) {
      error.value = message(err);
    }
  }

  return { intent, error, notes, busy, dialog, startKind, ask, cancel, confirm };
}
