<script setup lang="ts">
import type { KibanaDataView } from '@shared/domain/kibana';
import { useQuery } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import PickerPopover from '@workbench/components/pickers/PickerPopover.vue';
import { computed, ref } from 'vue';
import { control } from '../../../bridge/control';

const props = defineProps<{
  connectionId: string | null;
  enabled: boolean;
  /** Shown on the trigger until the list loads. */
  fallbackLabel: string | null;
}>();
const model = defineModel<string | null>({ default: null });
const emit = defineEmits<{ select: [view: KibanaDataView] }>();

const open = ref(false);
const search = ref('');

const query = useQuery(() => ({
  queryKey: ['kibana', 'dataViews', props.connectionId ?? ''] as const,
  queryFn: () => control.kibanaDataViews(props.connectionId ?? ''),
  enabled: !!props.connectionId && props.enabled,
  staleTime: 60_000,
}));

const options = computed(() =>
  (query.data.value ?? []).map((v) => ({
    value: v.id,
    label: v.name,
    detail: v.title === v.name ? undefined : v.title,
    testid: v.id,
  })),
);

function onSelect(option: { value: string }): void {
  const view = query.data.value?.find((v) => v.id === option.value);
  if (view) emit('select', view);
}
</script>

<template>
  <PickerPopover
    v-model="model"
    v-model:open="open"
    v-model:search="search"
    :options="options"
    :placeholder="t('studio.kibana.dataView.pick.placeholder')"
    :trigger-label="options.length === 0 ? (fallbackLabel ?? undefined) : undefined"
    :search-placeholder="t('studio.kibana.dataView.search.placeholder')"
    :empty-text="t('studio.kibana.dataView.none.empty')"
    :loading="query.isFetching.value && options.length === 0"
    :error="query.error.value ? String(query.error.value) : null"
    size="kira"
    trigger="toolbar"
    icon="database"
    testid="kibana-data-view-picker"
    @select="onSelect"
  />
</template>
