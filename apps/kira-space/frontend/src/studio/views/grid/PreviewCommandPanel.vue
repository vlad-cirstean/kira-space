<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SecondaryTabs, { type SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { PopoverContent } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import MonacoDiff from '@workbench/editor/MonacoDiff.vue';
import { computed, ref } from 'vue';
import MonacoHost from '../../editor/MonacoHost.vue';
import { useConnectionsStore } from '../../state/connections';
import { useTabsStore } from '../../state/tabs';
import { valueDiffSides } from '../shared/celleditor/valueDiff';
import { sqlDialectFor } from '../shared/sqlIdent';
import { getPage } from './page';
import { usePendingChangesStore } from './pendingChanges';

const props = defineProps<{ tabId: string }>();
const emit = defineEmits<{ close: [] }>();

const pendingChangesStore = usePendingChangesStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();

const view = ref<'changes' | 'sql'>('changes');
const cells = computed(() => pendingChangesStore.editedCells(props.tabId));
const hasEdits = computed(() => cells.value.length > 0);
const shownView = computed(() => (hasEdits.value ? view.value : 'sql'));
const viewItems = computed<SecondaryTabItem[]>(() => [
  {
    value: 'changes',
    label: t('space.ade.branchPanel.changes.label'),
    testid: 'preview-command-view-changes',
  },
  {
    value: 'sql',
    label: t('studio.cellEditor.format.sql.label'),
    testid: 'preview-command-view-sql',
  },
]);

const selectedId = ref<string | null>(null);
const selected = computed(
  () => cells.value.find((c) => c.id === selectedId.value) ?? cells.value[0],
);
const sides = computed(() => {
  const cellDiff = selected.value;
  if (!cellDiff) return null;
  const column = getPage(props.tabId)?.columns.find((c) => c.name === cellDiff.column);
  return valueDiffSides(cellDiff.before, cellDiff.after, {
    typeClass: column?.typeClass ?? 'text',
    dataType: column?.dataType ?? '',
    name: cellDiff.column,
  });
});

// P99 §5.5: a plain preview of the *currently staged* edits — nothing pushes a change event for
// it, and it must reflect this open's own pending state, so `staleTime: 0` (always refetch on
// mount) replaces the old ref([])/loading/error triple rather than the Infinity default.
const { data, isLoading, error } = useQuery(() => ({
  queryKey: ['previewPending', props.tabId] as const,
  queryFn: () => {
    const tab = tabsStore.findDataTab(props.tabId);
    if (!tab?.connectionId) return Promise.resolve([]);
    return pendingChangesStore.previewPending(tab.connectionId, tab.path, props.tabId);
  },
  enabled: !!tabsStore.findDataTab(props.tabId)?.connectionId && shownView.value === 'sql',
  // The key carries no pending-set version, so a stale cache entry from a
  // previous open would otherwise show for a moment (isLoading false while data exists) — this
  // component fully unmounts on close (DataView.vue's v-if="previewOpen"), so gcTime: 0 discards
  // the cache entry immediately and the next open starts empty until its own fetch lands.
  gcTime: 0,
  staleTime: 0,
}));

const statements = computed(() => data.value ?? []);
const errorMessage = computed(() => {
  const err = error.value;
  if (!err) return null;
  return errorText(err);
});

// P31 D40/F36: a blank line between statements — the trailing `;` on the last one is unchanged.
const doc = computed(() => statements.value.join(';\n\n') + (statements.value.length ? ';' : ''));

const sqlDialect = computed(() =>
  sqlDialectFor(
    connectionsStore.connectionRecord(tabsStore.findDataTab(props.tabId)?.connectionId)?.kind,
  ),
);

function close(): void {
  emit('close');
}
</script>

<template>
  <PopoverContent align="end" class="w-120 gap-0 p-0" data-testid="preview-command-panel">
    <div class="max-h-96 flex flex-col">
      <PanelHeader>
        <template #start><CodiconIcon name="code" :size="13" /></template>
        {{ t('docker.edit.pending.title') }}
        <template #actions>
          <TooltipIconButton
            icon="close"
            :label="t('common.action.close.label')"
            data-testid="preview-command-close"
            @click="close"
          />
        </template>
      </PanelHeader>
      <SecondaryTabs
        v-if="hasEdits"
        v-model="view"
        class="m-2 self-start"
        variant="segmented"
        :items="viewItems"
        :aria-label="t('studio.grid.previewCommand.view.aria')"
      />
      <template v-if="shownView === 'changes'">
        <ul
          class="flex max-h-24 flex-col overflow-auto"
          :aria-label="t('studio.grid.previewCommand.cells.aria')"
          data-testid="pending-diff-list"
        >
          <li v-for="c in cells" :key="c.id">
            <Button
              variant="toolbar"
              size="kira"
              class="justify-start"
              :aria-pressed="c.id === selected?.id"
              :data-column="c.column"
              data-testid="pending-diff-item"
              @click="selectedId = c.id"
            >
              <span class="font-data text-muted-foreground">{{ c.keyLabel }}</span>
              {{ c.column }}
              <template v-if="c.before === null || c.after === null">
                <Badge v-if="c.before === null">{{ t('studio.common.nullValue.label') }}</Badge>
                <span class="text-muted-foreground">→</span>
                <Badge v-if="c.after === null">{{ t('studio.common.nullValue.label') }}</Badge>
              </template>
            </Button>
          </li>
        </ul>
        <div v-if="sides" class="h-60">
          <MonacoDiff data-testid="pending-diff" v-bind="sides" />
        </div>
      </template>
      <InlineState v-else-if="isLoading" kind="loading">{{
        t('common.state.loading.loading')
      }}</InlineState>
      <InlineState v-else-if="errorMessage" kind="error" data-testid="preview-command-error">
        {{ errorMessage }}
      </InlineState>
      <InlineState v-else-if="statements.length === 0" kind="empty">{{
        t('studio.grid.previewCommand.empty')
      }}</InlineState>
      <div v-else class="h-60">
        <MonacoHost :doc="doc" language="sql" :sql-dialect="sqlDialect" readonly />
      </div>
    </div>
  </PopoverContent>
</template>
