<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
/**
 * §5.1.1's explicit "load more" affordance — pages of `pageSize` rows, never infinite scroll.
 * A plain click calls `graphView.loadMore()`; an Alt/Option-click ("do the bigger thing", the
 * workbench's own modifier convention) calls `graphView.loadAll()` instead. Both share one
 * `loading` state on `GraphViewState`, so a second press while either is in flight is a no-op —
 * the native `disabled` attribute plus the `loading` guard in `handlePress` cover it twice.
 *
 * Deliberately has no live region of its own: W14 owns "one polite live region" announcing both
 * load-more and refresh outcomes, and a second region here would fight it (plan lines ~1325-6).
 */
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { formatCount } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { ASYNC_FAILURES, type AsyncFailure } from '../state/asyncFailures.ts';
import type { GraphViewState } from '../state/graphView.ts';

const props = defineProps<{
  graphView: GraphViewState;
  pageSize: number;
  reportError: (err: unknown, failure: AsyncFailure) => void;
}>();

const isLoading = computed(() => props.graphView.loading.value !== 'idle');

const buttonLabel = computed(() => {
  const remaining = props.graphView.remaining.value;
  if (isLoading.value) {
    return t('git.loadMore.busy.label', { remaining: formatCount(remaining) });
  }
  if (remaining < props.pageSize) {
    return t('git.loadMore.last.label', { count: remaining });
  }
  return t('git.loadMore.page.label', {
    pageSize: formatCount(props.pageSize),
    remaining: formatCount(remaining),
  });
});

function handlePress(event: MouseEvent): void {
  if (isLoading.value) return;
  const load = event.altKey ? props.graphView.loadAll() : props.graphView.loadMore();
  load.catch((err: unknown) => props.reportError(err, ASYNC_FAILURES.loadMoreHistory));
}

function handleCancel(): void {
  props.graphView.cancelLoad();
}
</script>

<template>
  <!-- `remaining > 0` guards against the zero-chunk hole — a re-stream that emits no
       chunk leaves no server-side signal to correct, so this is the one place that hole can be
       closed. Kept visible while a load is in flight (`isLoading`) so Cancel does not vanish
       mid-load. -->
  <div
    v-if="!graphView.exhausted.value && (isLoading || graphView.remaining.value > 0)"
    class="flex items-center justify-center gap-1 py-1 px-2 shrink-0"
  >
    <TextTooltip :text="t('git.loadMore.loadAll.hint')">
      <Button variant="toolbar" size="kira" :pending="isLoading" @click="handlePress"
        ><CodiconIcon name="fold-down" :size="13" />
        {{ buttonLabel }}
      </Button>
    </TextTooltip>
    <Button
      v-if="isLoading"
      variant="toolbar"
      size="kira"
      :aria-label="t('git.loadMore.cancel.aria')"
      @click="handleCancel"
      ><CodiconIcon :name="ICON.stop" :size="13" />
      {{ t('common.action.cancel.label') }}
    </Button>
  </div>
</template>
