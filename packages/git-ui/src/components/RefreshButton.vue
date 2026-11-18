<script setup lang="ts">
/**
 * The refresh action: the leftmost operation button, `F5`. Calls
 * `GraphViewState.refresh()`, which already carries its own "second press while running is a
 * no-op" idempotency — the `disabled` attribute here is belt-and-braces, not the only
 * guard. Spins only while `loading === "refreshing"`, not for an unrelated `loadMore`/`loadAll`
 * in flight, so the icon never implies a re-walk that is not happening.
 *
 * A `repo.changed` event with `kind: "refsChanged"` shows a small dot and changes the tooltip,
 * cleared again whenever `loading` leaves `"refreshing"` — manual or automatic. The graph itself
 * auto-refreshes on this same signal, so this dot now exists purely as the *backup* for whatever
 * case genuinely did not auto-refresh, not as the primary mechanism.
 *
 * The `F5` keybinding itself is not attached here: `CommitGrid.vue` already owns a
 * keydown listener scoped to its own host element and emits a `refresh` event from it, which is
 * exactly "while the panel has focus" — precise focus scoping a second, independent
 * document-level listener here could not match without duplicating it. `App.vue` wires
 * that emit to this component's exposed `refresh()`, so both paths share one implementation and
 * one `hasPendingChange` state.
 */
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { computed, ref, watch } from 'vue';
import { ASYNC_FAILURES, type AsyncFailure } from '../state/asyncFailures.ts';
import type { GraphViewState } from '../state/graphView.ts';
import type { RepoState } from '../state/repo.ts';

const props = defineProps<{
  graphView: GraphViewState;
  repoState: RepoState;
  reportError: (err: unknown, failure: AsyncFailure) => void;
}>();

const REFRESH_CHORD = formatShortcut('view.refresh');
const hasPendingChange = ref(false);

watch(
  () => props.repoState.lastChange.value,
  (change) => {
    if (change?.kind === 'refsChanged') hasPendingChange.value = true;
  },
);

// Cleared on ANY completed refresh, not only one `doRefresh` itself triggered — D10's
// auto-refresh runs through the same `loading === 'refreshing'` state, and once it exists the
// dot must clear on that too or it becomes permanent noise.
watch(
  () => props.graphView.loading.value,
  (state, previous) => {
    if (previous === 'refreshing' && state !== 'refreshing') hasPendingChange.value = false;
  },
);

const isBusy = computed(() => props.graphView.loading.value !== 'idle');
const isRefreshing = computed(() => props.graphView.loading.value === 'refreshing');
const blockedByLoad = computed(() => isBusy.value && !isRefreshing.value);

async function doRefresh(): Promise<void> {
  if (isBusy.value) return;
  try {
    await props.graphView.refresh();
  } catch (err) {
    props.reportError(err, ASYNC_FAILURES.refresh);
  }
}

defineExpose({ refresh: doRefresh });
</script>

<template>
  <TooltipIconButton
    icon="refresh"
    :label="
      hasPendingChange ? t('git.toolbar.refreshPending.hint') : t('common.action.refresh.label')
    "
    :aria-label="t('common.action.refresh.label')"
    :kbd="hasPendingChange ? undefined : REFRESH_CHORD"
    class="relative"
    :pending="isRefreshing"
    :disabled="blockedByLoad"
    @click="doRefresh"
  >
    <template #badge>
      <StatusDot
        v-if="hasPendingChange"
        class="absolute top-0.5 right-0.5"
        tone="info"
        aria-hidden="true"
      />
    </template>
  </TooltipIconButton>
</template>
