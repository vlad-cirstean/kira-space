import { type MessageKey, t } from '@theme/i18n';
import { createSharedComposable } from '@vueuse/core';
import { computed } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import { useHostLabels } from '../hostLabels';
import type { TrackedUpdate } from '../wire';
import { meaningfulUpdates } from './meaningful';
import { useTrackedUpdates } from './queries';

function group(items: readonly TrackedUpdate[], key: (u: TrackedUpdate) => string) {
  const out = new Map<string, TrackedUpdate[]>();
  for (const u of items) {
    const k = key(u);
    if (k === '') continue;
    out.set(k, [...(out.get(k) ?? []), u]);
  }
  return out;
}

function build() {
  const query = useTrackedUpdates();
  const settings = useSettingsStore();
  const { shorts } = useHostLabels();

  const items = computed(() =>
    meaningfulUpdates(query.data.value?.updates ?? [], settings.tracking),
  );
  const byTask = computed(() => group(items.value, (u) => u.taskId));
  const byBranch = computed(() => group(items.value, (u) => u.branchId));

  function describe(u: TrackedUpdate): string {
    const changes = u.concepts
      .map((c) =>
        t(`space.ade.updates.${u.kind}.${c}.status` as MessageKey, { count: u.counts[c] ?? 1 }),
      )
      .join(', ');
    const subject = u.kind === 'changeRequest' ? `${shorts.value} ${u.label}` : u.label;
    return t('space.ade.updates.line.label', { subject, changes });
  }

  return { items, byTask, byBranch, describe };
}

export const useTaskUpdates = createSharedComposable(build);
