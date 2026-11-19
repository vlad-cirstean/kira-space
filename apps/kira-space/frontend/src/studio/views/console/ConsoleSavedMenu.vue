<script setup lang="ts">
import type { SavedConsoleQuery } from '@shared/domain/queries';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import TextPromptDialog from '@workbench/prompt/TextPromptDialog.vue';
import { useTextPrompt } from '@workbench/prompt/useTextPrompt';
import { computed } from 'vue';
import { useTabsStore } from '../../../state/tabs';
import { control } from '../../bridge/control';
import { guardedSavedWrite } from '../../state/guardedSavedWrite';
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
  mutationFn: (args: {
    entry: { id: string; name: string; pinned: boolean };
    patch: { pinned?: boolean; name?: string };
  }) =>
    guardedSavedWrite((guarded) =>
      control.queriesUpdate(
        args.entry.id,
        args.patch,
        guarded ? { name: args.entry.name, pinned: args.entry.pinned } : undefined,
      ),
    ),
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
const surface = useTaskAction();
const error = computed(
  () => surface.error.value ?? (savedQuery.error.value ? errorText(savedQuery.error.value) : null),
);

function apply(entry: SavedConsoleQuery): void {
  setText(props.tabId, entry.body.text);
  control.queriesTouch(entry.id).catch(() => {
    // Recency only: a failed touch changes nothing the user sees.
  });
  emit('close');
}

const togglePin = (entry: SavedConsoleQuery): Promise<boolean> =>
  surface.run(() => updateMutation.mutateAsync({ entry, patch: { pinned: !entry.pinned } }));
const rename = (entry: SavedConsoleQuery): Promise<boolean> =>
  surface.run(async () => {
    const name = await promptText(t('studio.console.savedMenu.renameTitle.title'), entry.name);
    if (!name || name.trim() === '') return;
    await updateMutation.mutateAsync({ entry, patch: { name: name.trim() } });
  });
const remove = (entry: SavedConsoleQuery): Promise<boolean> =>
  surface.run(() => deleteMutation.mutateAsync(entry.id));

const saveCurrent = (): Promise<boolean> =>
  surface.run(async () => {
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
    <template #below>
      <Alert v-if="error" variant="error" data-testid="console-saved-error">
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
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
    <template #footer>
      <Button variant="toolbar" size="kira" data-testid="console-save-current" @click="saveCurrent">
        <CodiconIcon name="add" :size="13" />
        {{ t('studio.console.savedMenu.saveCurrent.label') }}
      </Button>
    </template>
  </SavedListMenu>
</template>
