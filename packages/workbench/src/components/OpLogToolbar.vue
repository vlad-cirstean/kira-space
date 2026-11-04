<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { OpLogStatusFilter } from '../lib/opLog';

// The Operations panel's one toolbar row: filter text, status tabs (Running carries its count), the
// optional facet dropdown, the app's own `default` slot content and Clear.
const props = defineProps<{
  statusFilter: OpLogStatusFilter;
  runningCount: number;
  clearHint: string;
  facet?: { label: string; options: readonly { value: string; label: string }[] };
  facetValues: readonly string[];
}>();
const filterText = defineModel<string>('filterText', { required: true });
const emit = defineEmits<{
  'update:statusFilter': [value: OpLogStatusFilter];
  'update:facetValues': [value: string[]];
  clear: [];
}>();

const statusFilterOptions = computed(() => [
  { value: 'all', label: t('common.state.all.option'), testid: 'ops-tab-all' },
  {
    value: 'running',
    label: t('common.state.running.status'),
    count: props.runningCount > 0 ? props.runningCount : undefined,
    testid: 'ops-tab-running',
  },
  { value: 'error', label: t('workbench.opLog.errors.option'), testid: 'ops-tab-error' },
]);

function toggleFacet(value: string): void {
  emit(
    'update:facetValues',
    props.facetValues.includes(value)
      ? props.facetValues.filter((v) => v !== value)
      : [...props.facetValues, value],
  );
}
</script>

<template>
  <div class="shrink-0 flex items-center gap-2 py-1 px-2">
    <div class="w-40 shrink-0">
      <SearchField
        v-model="filterText"
        :placeholder="t('common.action.filter.label')"
        :aria-label="t('workbench.opLog.filter.aria')"
        data-testid="ops-filter"
      />
    </div>
    <SecondaryTabs
      variant="segmented"
      :model-value="statusFilter"
      :items="statusFilterOptions"
      @update:model-value="(v) => emit('update:statusFilter', v as OpLogStatusFilter)"
    />
    <DropdownMenu v-if="facet">
      <DropdownMenuTrigger as-child>
        <Button variant="toolbar" size="status" data-testid="ops-facet">
          <CodiconIcon name="filter" :size="13" />
          {{ facet.label }}
          <Badge v-if="facetValues.length > 0" data-testid="ops-facet-count">{{
            facetValues.length
          }}</Badge>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuItem data-testid="ops-facet-all" @select="emit('update:facetValues', [])">
          {{ t('common.state.all.option') }}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuCheckboxItem
          v-for="option in facet.options"
          :key="option.value"
          :model-value="facetValues.includes(option.value)"
          :data-testid="`ops-facet-${option.value}`"
          @select.prevent
          @update:model-value="toggleFacet(option.value)"
        >
          {{ option.label }}
        </DropdownMenuCheckboxItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <slot />
    <Tooltip>
      <TooltipTrigger as-child>
        <Button class="ml-auto" variant="dialog" size="kira-lg" @click="emit('clear')">{{
          t('common.action.clear.label')
        }}</Button>
      </TooltipTrigger>
      <TooltipContent>{{ clearHint }}</TooltipContent>
    </Tooltip>
  </div>
</template>
