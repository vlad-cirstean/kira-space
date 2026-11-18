<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { compareNatural, matchesQuery } from '@workbench/behaviour/match';
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import { useNetworks } from '../queries';
import { useDockerUiStore } from '../state/dockerUi';
import VirtualList from './VirtualList.vue';

const ui = useDockerUiStore();
const networks = useNetworks();

const rows = computed(() => {
  return (networks.data.value ?? [])
    .filter((n) => matchesQuery(n.name, ui.search))
    .map((network) => ({ key: network.id, network }))
    .sort((a, b) => compareNatural(a.network.name, b.network.name) || compareNatural(a.key, b.key));
});

function open(row: { network: { id: string } }): void {
  ui.select({ kind: 'network', id: row.network.id });
}
</script>

<template>
  <QueryState :query="networks" :is-empty="() => rows.length === 0">
    <template #empty>
      <EmptyState
        fill
        data-testid="docker-list-empty"
        icon="globe"
        :title="
          ui.search.trim() ? t('common.state.noMatches.empty') : t('docker.network.none.empty')
        "
        :description="ui.search.trim() ? t('docker.network.noMatches.emptyHint') : undefined"
      />
    </template>
    <VirtualList
      :rows="rows"
      keep-scroll-key="docker:networks"
      testid="docker-network-list"
      :select="open"
      :open="open"
    >
      <template #row="{ row, nav }">
        <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
        <div
          :class="
            cn(
              rowVariants({
                layout: 'tree',
                selected: ui.selection?.kind === 'network' && ui.selection.id === row.network.id,
              }),
              'group/row h-full gap-1.5',
            )
          "
          :style="rowIndent(0)"
          data-testid="docker-row"
          :data-id="row.network.id"
          role="option"
          :aria-selected="ui.selection?.kind === 'network' && ui.selection.id === row.network.id"
          v-bind="nav"
          :tabindex="nav.tabindex"
          @click="open(row)"
        >
          <CodiconIcon name="globe" :size="13" class="shrink-0 text-muted-foreground" />
          <span class="min-w-0 flex-1 truncate">{{ row.network.name }}</span>
          <span :class="cn('w-6 shrink-0 text-center', rowMeta)">{{
            row.network.containers > 0 ? row.network.containers : ''
          }}</span>
          <span class="w-14 shrink-0 truncate text-right" :class="rowMeta">{{
            row.network.driver
          }}</span>
        </div>
      </template>
    </VirtualList>
  </QueryState>
</template>
