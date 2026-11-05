<script setup lang="ts">
import {
  KIBANA_FILTER_OPERATORS,
  type KibanaField,
  type KibanaFilter,
  type KibanaFilterOperator,
  kibanaFilterOperatorSchema,
} from '@shared/domain/kibana';
import { useQuery } from '@tanstack/vue-query';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { NativeSelect } from '@theme/components/ui/native-select';
import { type MessageKey, t } from '@theme/i18n';
import PickerPopover from '@workbench/components/pickers/PickerPopover.vue';
import { computed, ref } from 'vue';

const props = defineProps<{
  fields: readonly KibanaField[];
  initial: KibanaFilter | null;
  /** Candidate values for a field, best effort. */
  suggest: (field: string) => Promise<string[]>;
  /** Connection, data view and request scope the suggestions depend on. */
  suggestScope: readonly string[];
}>();
const emit = defineEmits<{ save: [filter: KibanaFilter]; cancel: [] }>();

const OPERATOR_KEYS: Record<KibanaFilterOperator, MessageKey> = {
  is: 'studio.kibana.filter.operatorIs.option',
  is_not: 'studio.kibana.filter.operatorIsNot.option',
  is_one_of: 'studio.kibana.filter.operatorIsOneOf.option',
  is_not_one_of: 'studio.kibana.filter.operatorIsNotOneOf.option',
  exists: 'studio.kibana.filter.operatorExists.option',
  does_not_exist: 'studio.kibana.filter.operatorDoesNotExist.option',
  between: 'studio.kibana.filter.operatorBetween.option',
};

const field = ref(props.initial?.field ?? '');
const operator = ref<KibanaFilterOperator>(props.initial?.operator ?? 'is');
const text = ref(props.initial?.values.join(', ') ?? '');
const low = ref(props.initial?.operator === 'between' ? (props.initial.values[0] ?? '') : '');
const high = ref(props.initial?.operator === 'between' ? (props.initial.values[1] ?? '') : '');
const search = ref('');
const fieldOpen = ref(false);

const fieldOptions = computed(() =>
  props.fields
    .filter((f) => f.searchable)
    .map((f) => ({ value: f.name, label: f.name, detail: f.type, dataFont: true })),
);

const needsValues = computed(
  () => operator.value !== 'exists' && operator.value !== 'does_not_exist',
);
const isList = computed(() => operator.value === 'is_one_of' || operator.value === 'is_not_one_of');
const valuePlaceholder = computed(() =>
  isList.value ? t('studio.kibana.filter.valuesList.placeholder') : t('common.field.value.label'),
);
const isBetween = computed(() => operator.value === 'between');

const suggestions = useQuery(() => ({
  queryKey: ['kibana', 'filterValues', ...props.suggestScope, field.value] as const,
  queryFn: () => props.suggest(field.value),
  enabled: field.value !== '' && needsValues.value,
  staleTime: 30_000,
}));

const values = computed<string[]>(() => {
  if (isBetween.value) return [low.value.trim(), high.value.trim()];
  if (isList.value) {
    return text.value
      .split(',')
      .map((v) => v.trim())
      .filter((v) => v !== '');
  }
  return [text.value.trim()];
});

const valid = computed(() => {
  if (field.value === '') return false;
  if (!needsValues.value) return true;
  if (isBetween.value) return values.value.every((v) => v !== '');
  return values.value.length > 0 && values.value[0] !== '';
});

function onOperator(value: unknown): void {
  const parsed = kibanaFilterOperatorSchema.safeParse(value);
  if (parsed.success) operator.value = parsed.data;
}

function save(): void {
  if (!valid.value) return;
  emit('save', {
    field: field.value,
    operator: operator.value,
    values: needsValues.value ? values.value : [],
    disabled: props.initial?.disabled ?? false,
  });
}
</script>

<template>
  <form class="flex flex-col gap-2" data-testid="kibana-filter-editor" @submit.prevent="save">
    <PickerPopover
      v-model="field"
      v-model:open="fieldOpen"
      v-model:search="search"
      :options="fieldOptions"
      :placeholder="t('studio.kibana.filter.field.placeholder')"
      :search-placeholder="t('studio.kibana.filter.fieldSearch.placeholder')"
      :empty-text="t('studio.kibana.filter.noField.empty')"
      size="kira"
      testid="kibana-filter-field"
    />
    <NativeSelect
      variant="bordered"
      :model-value="operator"
      data-testid="kibana-filter-operator"
      @update:model-value="onOperator"
    >
      <option v-for="op in KIBANA_FILTER_OPERATORS" :key="op" :value="op">
        {{ t(OPERATOR_KEYS[op]) }}
      </option>
    </NativeSelect>
    <template v-if="needsValues">
      <div v-if="isBetween" class="flex items-center gap-1.5">
        <Input
          v-model="low"
          :aria-label="t('studio.kibana.filter.from.aria')"
          data-testid="kibana-filter-low"
        />
        <Input
          v-model="high"
          :aria-label="t('studio.kibana.filter.to.aria')"
          data-testid="kibana-filter-high"
        />
      </div>
      <template v-else>
        <Input
          v-model="text"
          list="kibana-filter-values"
          :placeholder="valuePlaceholder"
          :aria-label="t('studio.kibana.filter.value.aria')"
          data-testid="kibana-filter-value"
        />
        <datalist id="kibana-filter-values">
          <option v-for="v in suggestions.data.value ?? []" :key="v" :value="v" />
        </datalist>
      </template>
    </template>
    <div class="flex justify-end gap-1.5">
      <Button
        type="button"
        variant="toolbar"
        size="kira"
        data-testid="kibana-filter-cancel"
        @click="emit('cancel')"
      >
        {{ t('common.action.cancel.label') }}
      </Button>
      <Button
        type="submit"
        variant="dialog-primary"
        size="kira"
        :disabled="!valid"
        data-testid="kibana-filter-save"
      >
        {{ t('common.action.save.label') }}
      </Button>
    </div>
  </form>
</template>
