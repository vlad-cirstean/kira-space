<script setup lang="ts">
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useTabsStore } from '../../state/tabs';
import SavedListMenu from '../shared/SavedListMenu.vue';
import { type StreamFilterHistoryEntry, useStreamFilterHistoryStore } from './streamFilterHistory';

// Single-list sibling of FilterHistoryMenu: session-only history, no "Recent" section and no
// persisted saved tier (see streamFilterHistory.ts).
const props = defineProps<{ tabId: string }>();
const emit = defineEmits<{
  apply: [offset: string | null, partitions: number[], timestamp: string | null];
  close: [];
}>();

const streamFilterHistoryStore = useStreamFilterHistoryStore();
const tabsStore = useTabsStore();

function target(): { connectionId: string; path: string } | null {
  const tab = tabsStore.findStreamTab(props.tabId);
  return tab?.connectionId ? { connectionId: tab.connectionId, path: tab.path } : null;
}

const entries = computed<StreamFilterHistoryEntry[]>(() => {
  const dest = target();
  return dest ? streamFilterHistoryStore.listStreamFilterHistory(dest.connectionId, dest.path) : [];
});

function summarize(entry: StreamFilterHistoryEntry): string {
  const parts: string[] = [];
  if (entry.offset !== null)
    parts.push(t('studio.stream.history.offset.label', { value: entry.offset }));
  if (entry.partitions.length === 1)
    parts.push(t('studio.stream.partition.label', { id: String(entry.partitions[0]) }));
  else if (entry.partitions.length > 1)
    parts.push(t('studio.stream.history.partitions.label', { list: entry.partitions.join(', ') }));
  if (entry.timestamp !== null)
    parts.push(t('studio.stream.history.since.label', { value: entry.timestamp }));
  return parts.length > 0 ? parts.join(' · ') : t('studio.stream.emptyFilter.label');
}

function apply(entry: StreamFilterHistoryEntry): void {
  emit('apply', entry.offset, entry.partitions, entry.timestamp);
  emit('close');
}

function togglePin(entry: StreamFilterHistoryEntry): void {
  const dest = target();
  if (!dest) return;
  streamFilterHistoryStore.toggleStreamFilterHistoryPin(dest.connectionId, dest.path, entry.id);
}

function remove(entry: StreamFilterHistoryEntry): void {
  const dest = target();
  if (!dest) return;
  streamFilterHistoryStore.deleteStreamFilterHistoryEntry(dest.connectionId, dest.path, entry.id);
}
</script>

<template>
  <SavedListMenu
    :title="t('studio.common.filterHistory.placeholder')"
    :saved="entries"
    panel-testid="stream-filter-history"
    saved-entry-testid="stream-filter-history-entry"
    :empty-saved-text="t('studio.stream.noFilterHistory.empty')"
    :entry-text="summarize"
    @apply="apply"
    @toggle-pin="togglePin"
    @delete="remove"
  >
    <template #entry="{ entry }">
      <span class="min-w-0 flex-1 font-data"><TruncatedText :text="summarize(entry)" /></span>
    </template>
  </SavedListMenu>
</template>
