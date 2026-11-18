<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@theme/components/ui/command';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { computed, watch } from 'vue';
import { useCappedList } from '../../behaviour/cappedList';
import {
  foldOptions,
  type PickerGroup,
  type PickerOption,
  sectionize,
} from '../../lib/pickerOptions';
import ShowMoreButton from '../ShowMoreButton.vue';

// Internal to `pickers/`: the searchable, capped listbox under ProjectPicker and BranchPicker.
// Filtering and capping run here (`ignore-filter`), so rows capped out stay searchable.
const props = withDefaults(
  defineProps<{
    options: readonly PickerOption[];
    groups?: readonly PickerGroup[];
    /** Value of the chosen option; shows the check mark. */
    selected?: string | null;
    searchPlaceholder: string;
    emptyText: string;
    loading?: boolean;
    error?: string | null;
    /** Free-entry item for the typed query: its label, and a validation message or null. */
    customLabel?: (query: string) => string;
    validate?: (query: string) => string | null;
    testid: string;
    step?: number;
    listClass?: string;
  }>(),
  {
    groups: () => [],
    selected: null,
    loading: false,
    error: null,
    customLabel: undefined,
    validate: undefined,
    step: 50,
    listClass: undefined,
  },
);
const emit = defineEmits<{ select: [option: PickerOption]; custom: [text: string] }>();
const search = defineModel<string>('search', { default: '' });

const folded = computed(() => foldOptions(props.options, props.groups, search.value));
const { visible, hiddenCount, showMore, reset } = useCappedList(
  () => folded.value.ordered,
  props.step,
);
const sections = computed(() => sectionize(visible.value, props.groups));
watch(search, reset);
const typed = computed(() => search.value.trim());
const customError = computed(() =>
  typed.value === '' || !props.validate ? null : props.validate(typed.value),
);
const showCustom = computed(
  () => props.customLabel !== undefined && typed.value !== '' && !folded.value.exact,
);
const empty = computed(() => sections.value.length === 0 && !showCustom.value);

function pick(option: PickerOption): void {
  if (!option.disabled) emit('select', option);
}
function pickCustom(): void {
  if (customError.value === null) emit('custom', typed.value);
}
const idOf = (o: PickerOption): string => o.testid ?? o.value;
</script>

<template>
  <Command ignore-filter v-model:search="search">
    <CommandInput
      :placeholder="searchPlaceholder"
      :aria-label="searchPlaceholder"
      :data-testid="`${testid}-search`"
    />
    <slot name="top" />
    <div v-if="customError" class="px-2 pt-1">
      <FieldError :data-testid="`${testid}-error`">{{ customError }}</FieldError>
    </div>
    <CommandList :class="cn('max-h-72', listClass)">
      <CommandGroup v-if="showCustom">
        <CommandItem
          value="__custom"
          :disabled="customError !== null"
          :data-testid="`${testid}-custom`"
          @select="pickCustom"
        >
          <span class="min-w-0 truncate">{{ customLabel?.(typed) }}</span>
        </CommandItem>
      </CommandGroup>
      <CommandGroup
        v-for="section in sections"
        :key="section.id"
        :heading="section.label || undefined"
      >
        <CommandItem
          v-for="o in section.options"
          :key="o.value"
          :value="o.value"
          :disabled="o.disabled"
          :aria-description="o.reason"
          :data-checked="o.value === selected ? 'true' : 'false'"
          :data-testid="`${testid}-option-${idOf(o)}`"
          @select="pick(o)"
        >
          <span class="flex min-w-0 flex-1 flex-col">
            <span class="flex min-w-0 items-baseline gap-2">
              <span
                :class="cn('min-w-0 truncate', o.dataFont && 'font-data', o.labelClass)"
                :style="o.labelStyle"
                >{{
                  o.label
                }}</span
              >
              <span
                v-if="o.detail"
                class="ml-auto min-w-0 shrink-[2] truncate text-kira-sm text-muted-foreground"
                >{{
                  o.detail
                }}</span
              >
            </span>
            <span v-if="o.reason" class="text-kira-sm text-muted-foreground">{{ o.reason }}</span>
          </span>
        </CommandItem>
      </CommandGroup>
      <InlineState v-if="loading && empty" kind="loading">{{
        t('common.state.loading.loading')
      }}</InlineState>
      <InlineState v-else-if="error" kind="error">{{ error }}</InlineState>
      <InlineState v-else-if="empty && emptyText" kind="empty" :data-testid="`${testid}-empty`">{{
        emptyText
      }}</InlineState>
      <ShowMoreButton
        :hidden-count="hiddenCount"
        :step="step"
        :data-testid="`${testid}-more`"
        @click="showMore"
      />
    </CommandList>
  </Command>
</template>
