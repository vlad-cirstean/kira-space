<script setup lang="ts">
import type { KibanaField, KibanaFilter, KibanaFilterOperator } from '@shared/domain/kibana';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { ref } from 'vue';
import FilterEditor from './FilterEditor.vue';

defineProps<{
  fields: readonly KibanaField[];
  suggest: (field: string) => Promise<string[]>;
  suggestScope: readonly string[];
}>();
const filters = defineModel<KibanaFilter[]>({ required: true });

const NEGATED: Partial<Record<KibanaFilterOperator, KibanaFilterOperator>> = {
  is: 'is_not',
  is_not: 'is',
  is_one_of: 'is_not_one_of',
  is_not_one_of: 'is_one_of',
  exists: 'does_not_exist',
  does_not_exist: 'exists',
};

const adding = ref(false);
const editing = ref<number | null>(null);

function describe(f: KibanaFilter): string {
  const field = f.field;
  switch (f.operator) {
    case 'exists':
      return t('studio.kibana.filter.exists.label', { field });
    case 'does_not_exist':
      return t('studio.kibana.filter.doesNotExist.label', { field });
    case 'between':
      return t('studio.kibana.filter.clause.label', { field, value: f.values.join(' – ') });
  }
  const params = { field, value: f.values.join(', ') };
  return f.operator === 'is_not' || f.operator === 'is_not_one_of'
    ? t('studio.kibana.filter.negatedClause.label', params)
    : t('studio.kibana.filter.clause.label', params);
}

function replace(index: number, next: KibanaFilter): void {
  filters.value = filters.value.map((f, i) => (i === index ? next : f));
}
function remove(index: number): void {
  filters.value = filters.value.filter((_, i) => i !== index);
}
function onAdd(f: KibanaFilter): void {
  filters.value = [...filters.value, f];
  adding.value = false;
}
function onEdit(index: number, f: KibanaFilter): void {
  replace(index, f);
  editing.value = null;
}
function negate(index: number, f: KibanaFilter): void {
  const op = NEGATED[f.operator];
  if (op) replace(index, { ...f, operator: op });
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-1 px-2 pb-1" data-testid="kibana-filter-bar">
    <Badge
      v-for="(f, i) in filters"
      :key="`${i}-${f.field}-${f.operator}-${f.values.join(',')}`"
      variant="chip"
      class="h-control gap-0.5 bg-field pr-0.5 text-fg"
      :class="f.disabled && 'opacity-50'"
      data-testid="kibana-filter-pill"
    >
      <Popover :open="editing === i" @update:open="(o) => (editing = o ? i : null)">
        <PopoverTrigger as-child>
          <button
            type="button"
            :class="cn('max-w-64 truncate px-1', f.disabled && 'line-through')"
            data-testid="kibana-filter-edit"
          >
            {{ describe(f) }}
          </button>
        </PopoverTrigger>
        <PopoverContent align="start" class="w-80 p-2">
          <FilterEditor
            :fields="fields"
            :initial="f"
            :suggest="suggest"
            :suggest-scope="suggestScope"
            @save="onEdit(i, $event)"
            @cancel="editing = null"
          />
        </PopoverContent>
      </Popover>
      <TooltipIconButton
        :icon="f.disabled ? 'eye-closed' : 'eye'"
        :label="
          f.disabled
            ? t('studio.kibana.filter.enable.tooltip')
            : t('studio.kibana.filter.disable.tooltip')
        "
        data-testid="kibana-filter-toggle"
        @click="replace(i, { ...f, disabled: !f.disabled })"
      />
      <TooltipIconButton
        icon="circle-slash"
        :label="t('studio.kibana.filter.negate.tooltip')"
        :disabled="NEGATED[f.operator] === undefined"
        data-testid="kibana-filter-negate"
        @click="negate(i, f)"
      />
      <TooltipIconButton
        icon="close"
        :label="t('studio.kibana.filter.remove.tooltip')"
        data-testid="kibana-filter-remove"
        @click="remove(i)"
      />
    </Badge>
    <Popover v-model:open="adding">
      <PopoverTrigger as-child>
        <Button variant="toolbar" size="kira" data-testid="kibana-filter-add">
          <CodiconIcon name="add" :size="13" />
          {{ t('studio.kibana.filter.add.label') }}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" class="w-80 p-2">
        <FilterEditor
          v-if="adding"
          :fields="fields"
          :initial="null"
          :suggest="suggest"
          :suggest-scope="suggestScope"
          @save="onAdd"
          @cancel="adding = false"
        />
      </PopoverContent>
    </Popover>
  </div>
</template>
