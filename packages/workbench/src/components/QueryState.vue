<script setup lang="ts" generic="T">
import { isRetryable } from '@shared/domain/errorCodes';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import InlineState from '@theme/components/InlineState.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import type { Ref } from 'vue';
import { computed } from 'vue';
import { errorText, toCodedError } from '../bridge/codedError';

// The one loading / error / empty shell for bridge data. `pane` fills a panel or view body with an
// Empty block (spinner, error with Retry); `inline` is a one-line state inside a list or popover.
// Loading placeholders wait `SPINNER_DELAY` themselves. A caller passes its TanStack query result and fills the `default` slot with the loaded data.
interface QueryLike {
  data: Ref<T | undefined>;
  isPending: Ref<boolean>;
  isError: Ref<boolean>;
  error: Ref<unknown>;
  refetch: () => unknown;
}

const props = withDefaults(
  defineProps<{
    query: QueryLike;
    variant?: 'pane' | 'inline';
    /** Default: an empty array. */
    isEmpty?: (data: T) => boolean;
    loadingLabel?: string;
  }>(),
  { variant: 'pane', loadingLabel: () => t('common.state.loading.loading'), isEmpty: undefined },
);

defineSlots<{
  default(props: { data: T }): unknown;
  empty(): unknown;
  loading(): unknown;
  error(props: { message: string; retry: () => void }): unknown;
}>();

const message = computed(() => errorText(props.query.error.value));
const state = computed<'loading' | 'error' | 'empty' | 'data'>(() => {
  const { isPending, isError, data } = props.query;
  if (isError.value) return 'error';
  if (isPending.value || data.value === undefined) return 'loading';
  const empty = props.isEmpty
    ? props.isEmpty(data.value)
    : Array.isArray(data.value) && data.value.length === 0;
  return empty ? 'empty' : 'data';
});

const canRetry = computed(
  () => state.value === 'error' && isRetryable(toCodedError(props.query.error.value).code),
);

function retry(): void {
  void props.query.refetch();
}
</script>

<template>
  <template v-if="state === 'loading'">
    <slot name="loading">
      <EmptyState
        v-if="variant === 'pane'"
        fill
        loading
        data-testid="query-loading"
        :title="loadingLabel"
      />
      <InlineState v-else kind="loading" data-testid="query-loading">{{
        loadingLabel
      }}</InlineState>
    </slot>
  </template>
  <template v-else-if="state === 'error'">
    <slot name="error" :message="message" :retry="retry">
      <EmptyState
        v-if="variant === 'pane'"
        fill
        icon="warning"
        data-testid="query-error"
        :title="t('common.state.loadFailed.title')"
        :description="message"
      >
        <Button
          v-if="canRetry"
          size="kira"
          variant="toolbar"
          data-testid="query-retry"
          @click="retry"
          ><CodiconIcon :name="ICON.refresh" :size="13" />
          {{ t('common.action.retry.label') }}</Button
        >
      </EmptyState>
      <InlineState v-else kind="error" data-testid="query-error">{{ message }}</InlineState>
    </slot>
  </template>
  <slot v-else-if="state === 'empty'" name="empty" />
  <slot v-else :data="query.data.value as T" />
</template>
