<script setup lang="ts">
import { t } from '@theme/i18n';
import type { PickerOption } from '../../lib/pickerOptions';
import PickerPopover from './PickerPopover.vue';

// The one project picker: a git repository. Options come from the caller's own data source.
withDefaults(
  defineProps<{
    projects: readonly PickerOption[];
    placeholder: string;
    loading?: boolean;
    error?: string | null;
    disabled?: boolean;
    inline?: boolean;
    size?: 'kira' | 'kira-lg';
    trigger?: 'field' | 'toolbar';
    testid: string;
  }>(),
  {
    loading: false,
    error: null,
    disabled: false,
    inline: false,
    size: 'kira-lg',
    trigger: 'field',
  },
);
const emit = defineEmits<{ select: [option: PickerOption] }>();
const model = defineModel<string | null>({ default: null });
const open = defineModel<boolean>('open', { default: false });
</script>

<template>
  <PickerPopover
    v-model="model"
    v-model:open="open"
    :options="projects"
    :placeholder="placeholder"
    :search-placeholder="t('common.picker.findProject.placeholder')"
    :empty-text="t('common.picker.noProjects.empty')"
    :loading="loading"
    :error="error"
    :disabled="disabled"
    :inline="inline"
    :size="size"
    :trigger="trigger"
    :testid="testid"
    @select="emit('select', $event)"
  >
    <template #footer><slot name="footer" /></template>
  </PickerPopover>
</template>
