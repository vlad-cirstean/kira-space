<script setup lang="ts">
import type { KibanaField } from '@shared/domain/kibana';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { useCappedList } from '@workbench/behaviour/cappedList';
import { matchesQuery } from '@workbench/behaviour/match';
import ShowMoreButton from '@workbench/components/ShowMoreButton.vue';
import { computed, watch } from 'vue';
import { useConsoleUiStore } from '../viewState';

const props = defineProps<{
  tabId: string;
  fields: readonly KibanaField[];
  columns: readonly string[];
  loading: boolean;
}>();
const emit = defineEmits<{
  insert: [field: string];
  exists: [field: string];
  toggleColumn: [field: string];
}>();

const STEP = 200;
const ICONS: Record<string, string> = {
  string: 'symbol-string',
  number: 'symbol-numeric',
  date: 'calendar',
  boolean: 'symbol-boolean',
  ip: 'globe',
  geo_point: 'location',
};

// Search and "show more" outlive the component: a tab switch unmounts it.
const uiState = useConsoleUiStore().ui(props.tabId);
const search = computed({
  get: () => uiState.fieldSearch,
  set: (value: string) => {
    uiState.fieldSearch = value;
  },
});
const cap = computed({
  get: () => uiState.fieldCap ?? STEP,
  set: (value: number) => {
    uiState.fieldCap = value;
  },
});
const matches = computed(() => {
  return props.fields.filter((f) => matchesQuery(f.name, search.value));
});
const selected = computed(() =>
  props.columns.flatMap((c) => matches.value.filter((f) => f.name === c)),
);
const available = computed(() => matches.value.filter((f) => !props.columns.includes(f.name)));
const { visible, hiddenCount, showMore, reset } = useCappedList(() => available.value, STEP, cap);
watch(search, reset);
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="kibana-field-sidebar">
    <div class="p-2">
      <SearchField
        v-model="search"
        :placeholder="t('studio.kibana.fields.search.placeholder')"
        :aria-label="t('studio.kibana.fields.search.placeholder')"
        data-testid="kibana-field-search"
      />
    </div>
    <ul class="min-h-0 flex-1 overflow-y-auto">
      <template
        v-for="section in [
          { id: 'selected', rows: selected, heading: t('studio.kibana.fields.selected.label') },
          { id: 'available', rows: visible, heading: t('studio.kibana.fields.available.label') },
        ]"
        :key="section.id"
      >
        <li
          v-if="section.rows.length > 0 && (section.id === 'selected' || selected.length > 0)"
          class="px-2 pt-1 text-kira-sm font-medium text-muted-foreground"
          :data-testid="`kibana-${section.id}-fields`"
        >
          {{ section.heading }}
        </li>
        <li
          v-for="f in section.rows"
          :key="`${section.id}-${f.name}`"
          :class="[rowVariants({ layout: 'tree' }), 'group pl-2']"
          data-testid="kibana-field-row"
        >
          <Tooltip>
            <TooltipTrigger as-child>
              <button
                type="button"
                class="flex h-control min-w-0 flex-1 items-center gap-1.5 text-left text-kira-md"
                :data-testid="`kibana-field-${f.name}`"
                @click="emit('insert', f.name)"
              >
                <CodiconIcon
                  :name="ICONS[f.type] ?? 'symbol-field'"
                  :size="12"
                  class="shrink-0 text-muted-foreground"
                />
                <span class="truncate font-data">{{ f.name }}</span>
              </button>
            </TooltipTrigger>
            <TooltipContent>{{ f.type }}</TooltipContent>
          </Tooltip>
          <TooltipIconButton
            :icon="section.id === 'selected' ? 'close' : 'add'"
            class="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
            :label="
              t(
                section.id === 'selected'
                  ? 'studio.kibana.fields.column.remove.tooltip'
                  : 'studio.kibana.fields.column.add.tooltip',
                { field: f.name },
              )
            "
            :data-testid="`kibana-field-column-${f.name}`"
            @click="emit('toggleColumn', f.name)"
          />
          <TooltipIconButton
            icon="filter"
            class="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
            :label="t('studio.kibana.fields.exists.tooltip', { field: f.name })"
            :data-testid="`kibana-field-exists-${f.name}`"
            @click="emit('exists', f.name)"
          />
        </li>
      </template>
      <li
        v-if="!loading && matches.length === 0"
        class="px-2 py-1 text-kira-sm text-muted-foreground"
      >
        {{ t('studio.kibana.fields.none.empty') }}
      </li>
    </ul>
    <ShowMoreButton :hidden-count="hiddenCount" :step="STEP" @click="showMore" />
  </div>
</template>
