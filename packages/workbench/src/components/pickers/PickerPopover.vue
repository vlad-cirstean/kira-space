<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { cn } from '@theme/lib/utils';
import { computed, watch } from 'vue';
import type { PickerGroup, PickerOption } from '../../lib/pickerOptions';
import PickerCombobox from './PickerCombobox.vue';

// Internal to `pickers/`: trigger + popover (or the bare list when `inline`) around PickerCombobox.
// ProjectPicker and BranchPicker differ only in the defaults they pass here.
defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    options: readonly PickerOption[];
    groups?: readonly PickerGroup[];
    /** Trigger text while nothing is chosen. */
    placeholder: string;
    /** Trigger text instead of the chosen option's label (value not in `options`). */
    triggerLabel?: string;
    triggerDetail?: string;
    searchPlaceholder: string;
    emptyText: string;
    loading?: boolean;
    error?: string | null;
    customLabel?: (query: string) => string;
    validate?: (query: string) => string | null;
    disabled?: boolean;
    inline?: boolean;
    size?: 'kira' | 'kira-lg';
    trigger?: 'field' | 'toolbar';
    /** Codicon before the trigger text. */
    icon?: string;
    testid: string;
    step?: number;
  }>(),
  {
    groups: () => [],
    triggerLabel: undefined,
    triggerDetail: undefined,
    loading: false,
    error: null,
    customLabel: undefined,
    validate: undefined,
    disabled: false,
    inline: false,
    size: 'kira-lg',
    trigger: 'field',
    icon: undefined,
    step: 50,
  },
);
const emit = defineEmits<{ select: [option: PickerOption]; custom: [text: string] }>();
const model = defineModel<string | null>({ default: null });
const open = defineModel<boolean>('open', { default: false });
const search = defineModel<string>('search', { default: '' });

const chosen = computed(() => props.options.find((o) => o.value === model.value));
const label = computed(
  () => props.triggerLabel ?? chosen.value?.label ?? model.value ?? props.placeholder,
);
const placeholderShown = computed(
  () => props.triggerLabel === undefined && chosen.value === undefined && !model.value,
);

watch(open, (o) => {
  if (o) search.value = '';
});

function onSelect(option: PickerOption): void {
  model.value = option.value;
  open.value = false;
  emit('select', option);
}
function onCustom(text: string): void {
  open.value = false;
  emit('custom', text);
}
</script>

<template>
  <div v-if="inline" v-bind="$attrs" class="min-w-0">
    <PickerCombobox
      v-model:search="search"
      :options="options"
      :groups="groups"
      :selected="model"
      :search-placeholder="searchPlaceholder"
      :empty-text="emptyText"
      :loading="loading"
      :error="error"
      :custom-label="customLabel"
      :validate="validate"
      :testid="testid"
      :step="step"
      @select="onSelect"
      @custom="onCustom"
    >
      <template #top><slot name="top" /></template>
    </PickerCombobox>
    <slot name="footer" />
  </div>
  <Popover v-else v-model:open="open">
    <PopoverTrigger as-child>
      <PickerTrigger
        v-bind="$attrs"
        :size="size"
        :disabled="disabled"
        :data-testid="testid"
        :class="cn(trigger === 'toolbar' && 'border-transparent bg-transparent')"
      >
        <template #leading>
          <CodiconIcon v-if="icon" :name="icon" :size="12" />
        </template>
        <span
          :class="cn(chosen?.dataFont && 'font-data', placeholderShown && 'text-muted-foreground')"
          >{{
            label
          }}</span
        >
        <span v-if="triggerDetail" class="ml-1 text-kira-sm text-muted-foreground">{{
          triggerDetail
        }}</span>
      </PickerTrigger>
    </PopoverTrigger>
    <PopoverContent align="start" flush class="w-80" :data-testid="`${testid}-content`">
      <PickerCombobox
        v-model:search="search"
        :options="options"
        :groups="groups"
        :selected="model"
        :search-placeholder="searchPlaceholder"
        :empty-text="emptyText"
        :loading="loading"
        :error="error"
        :custom-label="customLabel"
        :validate="validate"
        :testid="testid"
        :step="step"
        @select="onSelect"
        @custom="onCustom"
      >
        <template #top><slot name="top" /></template>
      </PickerCombobox>
      <slot name="footer" />
    </PopoverContent>
  </Popover>
</template>
