<script setup lang="ts">
import {
  KIBANA_SAMPLE_SIZES,
  type KibanaConsoleState,
  type KibanaField,
  type KibanaSort,
} from '@shared/domain/kibana';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { formatCount } from '@theme/i18n/number';
import { computed, useId } from 'vue';

const props = defineProps<{ fields: readonly KibanaField[]; timeField: string | null }>();
const sort = defineModel<KibanaSort[]>('sort', { required: true });
const sampleSize = defineModel<KibanaConsoleState['sampleSize']>('sampleSize', { required: true });

const idBase = useId();
const DEFAULT = '';
const current = computed(() => sort.value[0] ?? null);
const sortable = computed(() =>
  props.fields.filter((f) => f.searchable && (f.aggregatable || f.type === 'date')),
);

function onField(value: unknown): void {
  const name = String(value);
  sort.value = name === DEFAULT ? [] : [{ field: name, order: current.value?.order ?? 'desc' }];
}

function flip(): void {
  const c = current.value;
  if (c) sort.value = [{ ...c, order: c.order === 'asc' ? 'desc' : 'asc' }];
}

const SAMPLE_SIZE_ITEMS = KIBANA_SAMPLE_SIZES.map((n) => ({
  value: String(n),
  label: formatCount(n),
  testid: `kibana-sample-size-${n}`,
}));

function onSize(value: unknown): void {
  const n = Number(value);
  const match = KIBANA_SAMPLE_SIZES.find((s) => s === n);
  if (match) sampleSize.value = match;
}
</script>

<template>
  <div class="flex items-center gap-1.5 px-2 py-1" data-testid="kibana-sort-control">
    <Field orientation="horizontal" class="w-auto">
      <FieldLabel :for="`${idBase}-field`">{{ t('studio.kibana.sort.field.label') }}</FieldLabel>
      <NativeSelect
        :id="`${idBase}-field`"
        variant="bordered"
        :model-value="current?.field ?? DEFAULT"
        data-testid="kibana-sort-field"
        @update:model-value="onField"
      >
        <option :value="DEFAULT">
          {{
            timeField
              ? t('studio.kibana.sort.timeField.option', { field: timeField })
              : t('studio.kibana.sort.score.option')
          }}
        </option>
        <option v-for="f in sortable" :key="f.name" :value="f.name">{{ f.name }}</option>
      </NativeSelect>
    </Field>
    <TooltipIconButton
      :icon="current?.order === 'asc' ? 'arrow-up' : 'arrow-down'"
      :label="
        current?.order === 'asc'
          ? t('studio.kibana.sort.ascending.tooltip')
          : t('studio.kibana.sort.descending.tooltip')
      "
      :disabled="!current"
      data-testid="kibana-sort-order"
      @click="flip"
    />
    <Field orientation="horizontal" class="ml-auto w-auto">
      <FieldLabel>{{ t('studio.kibana.sampleSize.label') }}</FieldLabel>
      <SecondaryTabs
        variant="choice"
        :aria-label="t('studio.kibana.sampleSize.label')"
        :model-value="String(sampleSize)"
        :items="SAMPLE_SIZE_ITEMS"
        data-testid="kibana-sample-size"
        @update:model-value="onSize"
      />
    </Field>
  </div>
</template>
