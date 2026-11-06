<script setup lang="ts">
import type { FilterHistoryEntry, SavedFilterQuery, SortSpec } from '@shared/domain/queries';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Separator } from '@theme/components/ui/separator';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import TextPromptDialog from '@workbench/prompt/TextPromptDialog.vue';
import { useTextPrompt } from '@workbench/prompt/useTextPrompt';
import { computed } from 'vue';
import { control } from '../../bridge/control';
import SavedListMenu from './SavedListMenu.vue';

// Generic across filter-bearing views (SQL's FilterToolbar, Document's filter row): "where"/"orderBy"
// name the two textual halves of whatever a view's filter row holds. Lives in views/shared/ so
// Document may import it (§11 forbids sideways view imports).
const props = defineProps<{
  connectionId: string | null;
  path: string;
  currentFilter: string | null;
  currentSort: SortSpec | null;
}>();
const emit = defineEmits<{ apply: [where: string | null, orderBy: SortSpec | null]; close: [] }>();

const queryClient = useQueryClient();
const listKey = computed(() => ['filterHistory', props.connectionId, props.path] as const);
const listQuery = useQuery(() => ({
  queryKey: listKey.value,
  queryFn: async () => {
    const connectionId = props.connectionId as string;
    const [saved, history] = await Promise.all([
      control.queriesList(connectionId, props.path),
      control.queriesHistoryList(connectionId, props.path, 20),
    ]);
    return { saved, history };
  },
  enabled: props.connectionId !== null,
}));

// Both lists share one literal union so the generic SavedListMenu infers a single Entry type.
type Entry = SavedFilterQuery | FilterHistoryEntry;
const savedEntries = computed<Entry[]>(() => listQuery.data.value?.saved ?? []);
const historyEntries = computed<Entry[]>(() => listQuery.data.value?.history ?? []);

const invalidate = () => queryClient.invalidateQueries({ queryKey: ['filterHistory'] });
const updateMutation = useMutation({
  mutationFn: (args: { id: string; patch: { pinned?: boolean; name?: string } }) =>
    control.queriesUpdate(args.id, args.patch),
  onSuccess: invalidate,
});
const deleteMutation = useMutation({
  mutationFn: (id: string) => control.queriesDelete(id),
  onSuccess: invalidate,
});
const saveMutation = useMutation({
  mutationFn: (args: { name: string; connectionId: string }) =>
    control.queriesSave({
      connectionId: args.connectionId,
      path: props.path,
      name: args.name,
      body: { where: props.currentFilter, orderBy: props.currentSort },
      pinned: false,
    }),
  onSuccess: invalidate,
});

// P107 T2-19: shared in-app substitute for window.prompt() (Electron's renderer doesn't implement
// it) — see packages/workbench/src/prompt/useTextPrompt.ts.
const {
  prompt: textPrompt,
  open: promptText,
  submit: submitPrompt,
  cancel: cancelPrompt,
} = useTextPrompt();

function sortLabel(orderBy: SortSpec | null): string | null {
  if (!orderBy) return null;
  if (orderBy.kind === 'text') return orderBy.text;
  return orderBy.terms.map((t) => `${t.column} ${t.direction.toUpperCase()}`).join(', ');
}

function summarize(where: string | null, orderBy: SortSpec | null): string {
  const parts: string[] = [];
  if (where) parts.push(`WHERE ${where}`);
  const orderText = sortLabel(orderBy);
  if (orderText) parts.push(`ORDER BY ${orderText}`);
  return parts.length > 0 ? parts.join(' / ') : t('studio.filterHistory.noFilter.label');
}

// `apply` fires for saved and recent rows alike; only saved entries carry a `body`.
function isSaved(entry: Entry): entry is SavedFilterQuery {
  return 'body' in entry;
}

async function applyEntry(entry: Entry): Promise<void> {
  if (isSaved(entry)) {
    emit('apply', entry.body.where, entry.body.orderBy);
    await control.queriesTouch(entry.id);
  } else {
    emit('apply', entry.where, entry.orderBy);
  }
  emit('close');
}

// Only saved entries have an id valid for these calls; a history entry never reaches the IPC.
async function togglePin(entry: Entry): Promise<void> {
  if (!isSaved(entry)) return;
  await updateMutation.mutateAsync({ id: entry.id, patch: { pinned: !entry.pinned } });
}
async function rename(entry: SavedFilterQuery): Promise<void> {
  const name = await promptText(t('studio.filterHistory.renamePrompt.title'), entry.name);
  if (!name || name.trim() === '') return;
  await updateMutation.mutateAsync({ id: entry.id, patch: { name: name.trim() } });
}
async function remove(entry: Entry): Promise<void> {
  if (!isSaved(entry)) return;
  await deleteMutation.mutateAsync(entry.id);
}

async function saveCurrent(): Promise<void> {
  if (!props.connectionId) return;
  const name = await promptText(t('studio.filterHistory.namePrompt.title'), '');
  if (!name || name.trim() === '') return;
  await saveMutation.mutateAsync({ name: name.trim(), connectionId: props.connectionId });
}
</script>

<template>
  <SavedListMenu
    :title="t('studio.filterHistory.saved.title')"
    :saved="savedEntries"
    :recent="historyEntries"
    panel-testid="filter-history"
    saved-entry-testid="saved-entry"
    recent-entry-testid="history-entry"
    :empty-saved-text="t('studio.project.menu.savedFilters.empty')"
    :empty-recent-text="t('studio.filterHistory.emptyRecent.empty')"
    :entry-text="(entry) => (isSaved(entry) ? entry.name : summarize(entry.where, entry.orderBy))"
    @apply="applyEntry"
    @toggle-pin="togglePin"
    @delete="remove"
  >
    <template #entry="{ entry }">
      <span class="min-w-0 flex-1" :class="{ 'font-data': !isSaved(entry) }"
        ><TruncatedText
          :text="isSaved(entry) ? entry.name : summarize(entry.where, entry.orderBy)"
        /></span
      >
    </template>
    <template #entry-actions="{ entry }">
      <TooltipIconButton
        v-if="isSaved(entry)"
        icon="edit"
        :label="t('common.action.rename.label')"
        @click.stop="rename(entry)"
      />
    </template>
    <template #footer>
      <Separator class="my-1" />
      <button
        type="button"
        :class="cn(rowVariants(), 'w-full text-primary')"
        data-testid="save-current-filter"
        @click="saveCurrent"
      >
        <span class="size-4 flex items-center justify-center shrink-0"
          ><CodiconIcon name="add" :size="13" /></span
        >
        {{ t('studio.filterHistory.saveCurrent.label') }}
      </button>
      <!-- Stays inside PopoverContent: a sibling would move focus outside its dismiss layer and
           close the menu before the prompt shows. -->
      <TextPromptDialog
        v-if="textPrompt"
        :title="textPrompt.title"
        :model-value="textPrompt.value"
        @update:model-value="(v) => textPrompt && (textPrompt.value = v)"
        @submit="submitPrompt"
        @close="cancelPrompt"
      />
    </template>
  </SavedListMenu>
</template>
