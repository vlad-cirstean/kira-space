<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import { Card } from '@theme/components/ui/card';
import { FieldError } from '@theme/components/ui/field';
import { formatCompactDuration } from '@theme/format';
import { t } from '@theme/i18n';
import { useIntervalFn } from '@vueuse/core';
import { computed, ref, watch } from 'vue';

// One status block for a worktree prepare script, shared by Kira Space's ADE and the git UI so a
// running, ready or failed script reads the same wherever it was started. The output (log lines)
// goes in the default slot; an action such as Retry goes in `actions`.
const props = defineProps<{
  state: 'running' | 'ready' | 'failed';
  title: string;
  /** Epoch ms the script started. */
  startedAt: number;
  /** Epoch ms it ended; absent while running. */
  finishedAt?: number | null;
  /** The configured timeout as text ("5m"), shown next to the elapsed time while running. */
  timeout?: string;
  /** Why it failed. */
  note?: string;
}>();

const now = ref(Date.now());
const { pause, resume } = useIntervalFn(() => (now.value = Date.now()), 1000, {
  immediate: props.state === 'running',
  immediateCallback: true,
});
watch(
  () => props.state,
  (s) => (s === 'running' ? resume() : pause()),
);

const elapsed = computed(() => {
  if (props.state === 'running') return formatCompactDuration(now.value - props.startedAt);
  return props.finishedAt == null ? '' : formatCompactDuration(props.finishedAt - props.startedAt);
});
</script>

<template>
  <Card :data-state="state" data-testid="script-progress">
    <div class="flex min-w-0 items-center gap-2 text-kira-md">
      <Spinner
        v-if="state === 'running'"
        :size="12"
        class="text-info"
        :label="t('common.state.running.status')"
      />
      <CodiconIcon v-else-if="state === 'ready'" name="check" :size="13" class="shrink-0 text-ok" />
      <CodiconIcon v-else name="error" :size="13" class="shrink-0 text-error" />
      <span class="min-w-0 truncate font-medium" data-testid="script-progress-title">{{
        title
      }}</span>
      <span
        v-if="elapsed"
        class="shrink-0 font-data text-kira-sm text-muted-foreground"
        data-testid="script-progress-elapsed"
      >
        {{
          state === 'running' && timeout
            ? t('common.ofTotal.label', { value: elapsed, total: timeout })
            : elapsed
        }}
      </span>
      <span class="flex-1" />
      <slot name="actions" />
    </div>
    <FieldError
      v-if="state === 'failed' && note"
      class="break-words"
      data-testid="script-progress-reason"
    >
      {{ note }}
    </FieldError>
    <slot />
  </Card>
</template>
