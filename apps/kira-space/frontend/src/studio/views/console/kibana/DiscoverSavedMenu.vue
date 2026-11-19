<script setup lang="ts">
import type { SavedDiscoverQuery } from '@shared/domain/queries';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { errorText } from '@workbench/bridge/codedError';
import TextPromptDialog from '@workbench/prompt/TextPromptDialog.vue';
import { useTextPrompt } from '@workbench/prompt/useTextPrompt';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import { useTabsStore } from '../../../../state/tabs';
import { control } from '../../../bridge/control';
import { guardedSavedWrite } from '../../../state/guardedSavedWrite';
import SavedListMenu from '../../shared/SavedListMenu.vue';
import { setKibana, setText } from '../state';
import { kibanaStateFromBody, relinkName, unlink } from './savedDiscover';

// Saved Discover queries of one connection on the shared SavedListMenu shell.
const props = defineProps<{
  connectionId: string;
  linkedId: string | null;
  dirty: boolean;
}>();
// `apply` fires once any discard confirmation passed; the toolbar owns restoring and saving.
const emit = defineEmits<{ close: []; apply: [entry: SavedDiscoverQuery]; saveCurrent: [] }>();

const queryClient = useQueryClient();
const savedQuery = useQuery(() => ({
  queryKey: ['kibanaSaved', props.connectionId] as const,
  queryFn: () => control.queriesListDiscover(props.connectionId),
  enabled: !!props.connectionId,
}));
const saved = computed<SavedDiscoverQuery[]>(() => savedQuery.data.value ?? []);

const confirmDialogStore = useConfirmDialogStore();
const {
  prompt: textPrompt,
  open: promptText,
  submit: submitPrompt,
  cancel: cancelPrompt,
} = useTextPrompt();

const invalidate = () => queryClient.invalidateQueries({ queryKey: ['kibanaSaved'] });
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

const onApply = (entry: SavedDiscoverQuery): Promise<void> =>
  attempt(async () => {
    if (props.dirty) {
      const ok = await confirmDialogStore.confirm({
        title: t('studio.kibana.saved.discard.title'),
        message: t('studio.kibana.saved.discard.body'),
        confirmLabel: t('common.action.discard.label'),
        tone: 'danger',
      });
      if (!ok) return;
    }
    emit('apply', entry);
    control.queriesTouch(entry.id).catch(() => {
      // Recency only: a failed touch changes nothing the user sees.
    });
    emit('close');
  });

const togglePin = (entry: SavedDiscoverQuery): Promise<void> =>
  attempt(() => updateMutation.mutateAsync({ entry, patch: { pinned: !entry.pinned } }));

const rename = (entry: SavedDiscoverQuery): Promise<void> =>
  attempt(async () => {
    const name = await promptText(t('studio.console.savedMenu.renameTitle.title'), entry.name);
    if (!name || name.trim() === '') return;
    // Undefined: the user declined to overwrite another window's change; nothing was renamed.
    if (!(await updateMutation.mutateAsync({ entry, patch: { name: name.trim() } }))) return;
    relinkName(entry.id, name.trim());
  });

const remove = (entry: SavedDiscoverQuery): Promise<void> =>
  attempt(async () => {
    const ok = await confirmDialogStore.confirm({
      title: t('studio.kibana.saved.delete.title'),
      message: t('studio.kibana.saved.delete.body', { name: entry.name }),
      confirmLabel: t('common.action.delete.label'),
      tone: 'danger',
    });
    if (!ok) return;
    await deleteMutation.mutateAsync(entry.id);
    unlink(entry.id);
  });

const openInNewTab = (entry: SavedDiscoverQuery): Promise<void> =>
  attempt(async () => {
    const opened = useTabsStore().openDiscoverTab(props.connectionId, '', { newTab: true });
    if (!opened) return;
    setText(opened.id, entry.body.query);
    setKibana(opened.id, kibanaStateFromBody(entry.body, entry));
    control.queriesTouch(entry.id).catch(() => {
      // Recency only.
    });
    emit('close');
  });

// The name prompt lives in the toolbar, so the popover closes first.
function onSaveCurrent(): void {
  emit('close');
  emit('saveCurrent');
}
</script>

<template>
  <SavedListMenu
    :title="t('studio.console.savedMenu.title')"
    :saved="saved"
    panel-testid="kibana-saved-menu"
    saved-entry-testid="kibana-saved-entry"
    :empty-saved-text="t('studio.console.savedMenu.empty')"
    :entry-text="(entry) => entry.name"
    @apply="onApply"
    @toggle-pin="togglePin"
    @delete="remove"
  >
    <template #entry="{ entry }">
      <span class="flex min-w-0 flex-1 flex-col">
        <span class="flex items-center gap-1">
          <TruncatedText :text="entry.name" />
          <CodiconIcon
            v-if="entry.id === linkedId"
            name="check"
            :size="12"
            class="shrink-0 text-muted-foreground"
            data-testid="kibana-saved-linked"
          />
        </span>
        <span class="truncate text-kira-sm text-muted-foreground">{{
          entry.body.dataViewName ?? entry.body.dataViewId
        }}</span>
      </span>
    </template>
    <template #entry-actions="{ entry }">
      <TooltipIconButton
        :icon="ICON.openInNewTab"
        :label="t('common.action.openInNewTab.label')"
        data-testid="kibana-saved-open-new"
        @click.stop="openInNewTab(entry)"
      />
      <TooltipIconButton
        icon="edit"
        :label="t('common.action.rename.label')"
        data-testid="kibana-saved-rename"
        @click.stop="rename(entry)"
      />
    </template>
    <template #below>
      <Alert v-if="error" variant="error" data-testid="kibana-saved-menu-error">
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
      <Button
        variant="toolbar"
        size="kira"
        data-testid="kibana-saved-save-current"
        @click="onSaveCurrent"
      >
        <CodiconIcon name="add" :size="13" />
        {{ t('studio.kibana.saved.saveCurrent.label') }}
      </Button>
    </template>
  </SavedListMenu>
</template>
