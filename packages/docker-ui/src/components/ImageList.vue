<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatBytes } from '@workbench/behaviour/format';
import { compareNatural, matchesQuery } from '@workbench/behaviour/match';
import Identifier from '@workbench/components/Identifier.vue';
import QueryState from '@workbench/components/QueryState.vue';
import VirtualList from '@workbench/components/VirtualList.vue';
import { computed } from 'vue';
import { shortId } from '../lib/format';
import { useImages } from '../queries';
import { useDockerUiStore } from '../state/dockerUi';

const ui = useDockerUiStore();
const images = useImages();

const rows = computed(() => {
  return (images.data.value ?? [])
    .filter((i) => matchesQuery(i.id, ui.search) || i.tags.some((t) => matchesQuery(t, ui.search)))
    .map((image) => ({ key: image.id, image, title: image.tags[0] ?? '<none>' }))
    .sort((a, b) => compareNatural(a.title, b.title) || compareNatural(a.key, b.key));
});

function open(row: { image: { id: string } }): void {
  ui.select({ kind: 'image', id: row.image.id });
}
</script>

<template>
  <QueryState :query="images" :is-empty="() => rows.length === 0">
    <template #empty>
      <EmptyState
        fill
        data-testid="docker-list-empty"
        icon="package"
        :title="ui.search.trim() ? t('common.state.noMatches.empty') : t('docker.image.none.empty')"
        :description="
          ui.search.trim()
            ? t('docker.image.noMatches.emptyHint')
            : t('docker.image.none.emptyHint')
        "
      />
    </template>
    <VirtualList
      :rows="rows"
      keep-scroll-key="docker:images"
      testid="docker-image-list"
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
                selected: ui.selection?.kind === 'image' && ui.selection.id === row.image.id,
              }),
              'group/row h-full gap-1.5',
            )
          "
          :style="rowIndent(0)"
          data-testid="docker-row"
          :data-id="row.image.id"
          role="option"
          :aria-selected="ui.selection?.kind === 'image' && ui.selection.id === row.image.id"
          v-bind="nav"
          :tabindex="nav.tabindex"
          @click="open(row)"
        >
          <CodiconIcon name="package" :size="13" class="shrink-0 text-muted-foreground" />
          <span class="min-w-0 flex-1 truncate">
            {{ row.title }}
            <span v-if="row.image.tags.length > 1" class="text-kira-sm text-muted-foreground"
              >+{{ row.image.tags.length - 1 }}</span
            >
            <Identifier
              v-if="row.image.dangling"
              :value="row.image.id"
              :short="shortId"
              :class="rowMeta"
            />
          </span>
          <span :class="cn('w-6 shrink-0 text-center', rowMeta)">{{
            row.image.containers > 0 ? row.image.containers : ''
          }}</span>
          <span class="w-14 shrink-0 text-right font-data" :class="rowMeta">{{
            formatBytes(row.image.size)
          }}</span>
        </div>
      </template>
    </VirtualList>
  </QueryState>
</template>
