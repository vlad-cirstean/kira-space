<script setup lang="ts">
import type { SavedConsoleQuery } from '@shared/domain/queries';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Separator } from '@theme/components/ui/separator';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { errorText } from '@workbench/bridge/codedError';
import TextPromptDialog from '@workbench/prompt/TextPromptDialog.vue';
import { useTextPrompt } from '@workbench/prompt/useTextPrompt';
import { computed, ref } from 'vue';
import { control } from '../../bridge/control';
import { useTabsStore } from '../../state/tabs';
import SavedListMenu from '../shared/SavedListMenu.vue';
import { setText } from './state';

// Saved-only sibling of FilterHistoryMenu (§8.14: the console has no run history), scoped to one
// tab's connectionId/path, on the shared SavedListMenu shell.
const props = defineProps<{ tabId: string }>();
const emit = defineEmits<{ close: [] }>();

function tab() {
  return useTabsStore().findConsoleTab(props.tabId);
}

const queryClient = useQueryClient();
const savedQuery = useQuery(() => {
  const current = tab();
  return {
    queryKey: ['consoleSaved', current?.connectionId ?? null, current?.path ?? null] as const,
    queryFn: () =>
      control.queriesListConsole(current?.connectionId as string, current?.path as string),
    enabled: !!current?.connectionId,
  };
});
const saved = computed<SavedConsoleQuery[]>(() => savedQuery.data.value ?? []);

// P107 T2-19: shared in-app substitute for window.prompt() — see
// packages/workbench/src/prompt/useTextPrompt.ts (same one FilterHistoryMenu.vue uses).
const {
  prompt: textPrompt,
  open: promptText,
  submit: submitPrompt,
  cancel: cancelPrompt,
} = useTextPrompt();

const invalidate = () => queryClient.invalidateQueries({ queryKey: ['consoleSaved'] });
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
  mutationFn: (args: { name: string; text: string; connectionId: string; path: string }) =>
    control.queriesSaveConsole({
      connectionId: args.connectionId,
      path: args.path,
      name: args.name,
      body: { text: args.text },
      pinned: false,
    }),
  onSuccess: invalidate,
});
// A storage failure shows in the footer, never as an unhandled rejection with nothing on screen.
const failure = ref<string | null>(null);
const error = computed(
  () => failure.value ?? (savedQuery.error.value ? errorText(savedQuery.error.value) : null),
);
async function attempt(action: () => Promise<unknown>): Promise<void> {
  failure.value = null;
  try {
    await action();
  } catch (err) {
    failure.value = errorText(err);
  }
}

function apply(entry: SavedConsoleQuery): void {
  setText(props.tabId, entry.body.text);
  control.queriesTouch(entry.id).catch(() => {
    // Recency only: a failed touch changes nothing the user sees.
  });
  emit('close');
}

const togglePin = (entry: SavedConsoleQuery): Promise<void> =>
  attempt(() => updateMutation.mutateAsync({ id: entry.id, patch: { pinned: !entry.pinned } }));
const rename = (entry: SavedConsoleQuery): Promise<void> =>
  attempt(async () => {
    const name = await promptText(t('studio.console.savedMenu.renameTitle.title'), entry.name);
    if (!name || name.trim() === '') return;
    await updateMutation.mutateAsync({ id: entry.id, patch: { name: name.trim() } });
  });
const remove = (entry: SavedConsoleQuery): Promise<void> =>
  attempt(() => deleteMutation.mutateAsync(entry.id));

const saveCurrent = (): Promise<void> =>
  attempt(async () => {
    const current = tab();
    if (!current?.connectionId) return;
    const name = await promptText(t('studio.console.savedMenu.nameTitle.title'), '');
    if (!name || name.trim() === '') return;
    await saveMutation.mutateAsync({
      name: name.trim(),
      text: current.state.text,
      connectionId: current.connectionId,
      path: current.path,
    });
  });
</script>

<template>
  <SavedListMenu
    :title="t('studio.console.savedMenu.title')"
    :saved="saved"
    panel-testid="console-saved-menu"
    saved-entry-testid="console-saved-entry"
    :empty-saved-text="t('studio.console.savedMenu.empty')"
    :entry-text="(entry) => entry.name"
    @apply="apply"
    @toggle-pin="togglePin"
    @delete="remove"
  >
    <template #entry="{ entry }">
      <span class="min-w-0 flex-1"><TruncatedText :text="entry.name" /></span>
    </template>
    <template #entry-actions="{ entry }">
      <TooltipIconButton
        icon="edit"
        :label="t('common.action.rename.label')"
        @click.stop="rename(entry)"
      />
    </template>
    <template #footer>
      <Separator class="my-1" />
      <Alert v-if="error" variant="error" data-testid="console-saved-error">
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <button
        type="button"
        :class="cn(rowVariants(), 'w-full text-primary')"
        data-testid="console-save-current"
        @click="saveCurrent"
      >
        <span class="size-4 flex items-center justify-center shrink-0"
          ><CodiconIcon name="add" :size="13" /></span
        >
        {{ t('studio.console.savedMenu.saveCurrent.label') }}
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
