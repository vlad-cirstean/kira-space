<script setup lang="ts" generic="T">
import type { UseQueryReturnType } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import DetailSection from '@theme/components/DetailSection.vue';
import InlineState from '@theme/components/InlineState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import { computed } from 'vue';

// A section measured only on demand (`enabled: false` query): the taken-at time and a re-measure
// button in the actions, then loading, not measured with its Measure button, or the data. A first
// failure is an InlineState; a failed re-measure keeps the data and adds an Alert.
const props = defineProps<{
  title: string;
  /** Test id prefix: `-taken`, `-refresh`, `-error`, `-measuring`, `-measure`. */
  testid: string;
  query: UseQueryReturnType<T | undefined, Error>;
  takenAt: (data: T) => string;
  measureTooltip: string;
  notMeasured: string;
}>();
defineSlots<{ default(props: { data: T }): unknown }>();

const data = computed(() => props.query.data.value);
const measuring = computed(() => props.query.isFetching.value);
const error = computed(() => props.query.error.value);
const measure = (): void => {
  void props.query.refetch();
};
</script>

<template>
  <DetailSection :title="title">
    <template #actions>
      <span class="ml-auto flex items-center gap-1">
        <span v-if="data !== undefined" :data-testid="`${testid}-taken`"
          ><RelativeTime :at="takenAt(data)" /></span
        >
        <TooltipIconButton
          :icon="ICON.refresh"
          :label="measureTooltip"
          :disabled="measuring"
          :data-testid="`${testid}-refresh`"
          @click="measure"
        />
      </span>
    </template>

    <template v-if="error">
      <Alert v-if="data !== undefined" variant="error" :data-testid="`${testid}-error`">
        <AlertDescription>{{ error.message }}</AlertDescription>
      </Alert>
      <InlineState v-else kind="error" :data-testid="`${testid}-error`">{{
        error.message
      }}</InlineState>
    </template>

    <InlineState
      v-if="measuring && data === undefined"
      kind="loading"
      :data-testid="`${testid}-measuring`"
      >{{
        t('docker.measuring.loading')
      }}</InlineState
    >
    <div v-else-if="data === undefined" class="flex flex-wrap items-center gap-3">
      <InlineState kind="empty">{{ notMeasured }}</InlineState>
      <Button size="kira" variant="toolbar" :data-testid="`${testid}-measure`" @click="measure"
        ><CodiconIcon name="dashboard" :size="13" />{{ t('docker.measure.label') }}</Button
      >
    </div>
    <slot v-else :data="data" />
  </DetailSection>
</template>
