<script setup lang="ts">
import { t } from '@theme/i18n';
import PickerPopover from '@workbench/components/pickers/PickerPopover.vue';
import type { PickerOption } from '@workbench/lib/pickerOptions';
import { computed, ref } from 'vue';
import type { KubeNamespaces } from '../wire';

// `value` is '' for all namespaces. A restricted account can't list namespaces: the picker then
// offers the fallback and any typed name.
const props = defineProps<{ value: string; data: KubeNamespaces | undefined }>();
const emit = defineEmits<{ change: [namespace: string] }>();

const ALL = '';
const open = ref(false);

const options = computed<PickerOption[]>(() => {
  const names = props.data?.namespaces ?? [];
  const named = names.map((n) => ({ value: n, label: n, dataFont: true }));
  if (props.data?.restricted) {
    return props.data.fallback
      ? [{ value: props.data.fallback, label: props.data.fallback, dataFont: true }]
      : [];
  }
  return [{ value: ALL, label: t('kubernetes.namespace.all.option') }, ...named];
});

const restricted = computed(() => props.data?.restricted === true);
</script>

<template>
  <PickerPopover
    v-model:open="open"
    :model-value="value"
    :options="options"
    :placeholder="t('studio.common.nodeKind.namespace.label')"
    :trigger-label="value === ALL ? t('kubernetes.namespace.all.option') : value"
    :search-placeholder="t('kubernetes.namespace.picker.search.placeholder')"
    :empty-text="
      restricted ? t('kubernetes.namespace.restricted.hint') : t('common.state.noMatches.empty')
    "
    :custom-label="restricted ? (q) => t('common.picker.useValue.label', { value: q }) : undefined"
    size="kira"
    trigger="toolbar"
    icon="symbol-namespace"
    testid="kube-namespace-picker"
    @select="(o) => emit('change', o.value)"
    @custom="(text) => emit('change', text.trim())"
  />
</template>
