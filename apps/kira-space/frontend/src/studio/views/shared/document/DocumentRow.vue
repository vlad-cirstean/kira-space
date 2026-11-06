<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatBytes } from '@workbench/behaviour/format';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import type { DocumentRowView } from './rows';

// The Mongo document row's head — down to its five shared data-testids — duplicated
// between DocumentView.vue and ConsoleResultGrid.vue's own read-only copy. The expansion *state*
// behind `expanded` stays genuinely different per caller (documents: persisted, default-expanded;
// console: runtime-only, default-collapsed) — this component only renders the
// row that reads it. `rowScope` names the tab id or console result key the caller resolved `view`
// against (rows.ts's own registered-source key), for a caller that needs it alongside `view.id`.
// Named `rowScope`, not `scope` — the HTML global `scope` attribute name collides with
// Biome's `noHeaderScope`, which reads the attribute name off every call site that binds it.
defineProps<{
  navProps?: { 'data-nav-key': string; tabindex: number; onFocus: () => void };
  view: DocumentRowView;
  rowScope: string;
  expanded: boolean;
  selected: boolean;
  searchMatch: boolean;
  searchMatchCurrent: boolean;
}>();

const emit = defineEmits<{ toggle: []; select: [] }>();

// The container's list navigation claims Enter and the arrows; Space mirrors a single click. The
// expand-toggle button and the #actions slot's controls stay their own tab stops.
function onHeadKeydown(e: KeyboardEvent): void {
  if (e.key !== ' ') return;
  e.preventDefault();
  emit('select');
}
</script>

<template>
  <div
    class="doc-row flex flex-col"
    :class="{
      'search-match bg-search-match': searchMatch,
      'search-match-current bg-search-match-current': searchMatchCurrent,
    }"
    :data-id="view.id"
  >
    <div
      class="doc-head"
      :class="
        cn(
          rowVariants({ layout: 'tree', selected: expanded }),
          'shrink-0 gap-1.5 px-2',
          selected && 'shadow-[inset_2px_0_0_var(--primary)]',
        )
      "
      role="option"
      v-bind="navProps"
      :tabindex="navProps?.tabindex ?? 0"
      :aria-selected="selected"
      @click="$emit('select')"
      @keydown="onHeadKeydown"
    >
      <TreeTwisty
        :expanded="expanded"
        has-children
        testid="document-toggle-expand"
        @toggle="$emit('toggle')"
      />
      <span
        class="shrink-0 max-w-56 truncate text-fg text-kira-md font-data"
        data-testid="document-id"
        >{{
          view.idLabel
        }}</span
      >
      <Badge data-testid="document-field-count">{{
        t('studio.documents.row.fieldCount.status', { count: view.fieldCount })
      }}</Badge>
      <Badge data data-testid="document-byte-badge">{{ view.byteLabel }}</Badge>
      <Tooltip v-if="view.isTruncated">
        <TooltipTrigger as-child>
          <Badge variant="warn" data-testid="document-truncated">{{
            t('studio.common.truncated.status')
          }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.common.cellTruncated.tooltip', { size: formatBytes(64 * 1024) })
        }}</TooltipContent>
      </Tooltip>
      <slot name="actions" />
    </div>
    <slot name="body" />
  </div>
</template>
