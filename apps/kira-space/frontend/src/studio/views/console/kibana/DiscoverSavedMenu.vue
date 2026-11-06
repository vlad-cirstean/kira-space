<script setup lang="ts">
import type { SavedDiscoverQuery } from '@shared/domain/queries';
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
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import { control } from '../../../bridge/control';
import { useTabsStore } from '../../../state/tabs';
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
  mutationFn: (args: { id: string; patch: { pinned?: boolean; name?: string } }) =>
    control.queriesUpdate(args.id, args.patch),
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
  attempt(() => updateMutation.mutateAsync({ id: entry.id, patch: { pinned: !entry.pinned } }));

const rename = (entry: SavedDiscoverQuery): Promise<void> =>
  attempt(async () => {
    const name = await promptText(t('studio.console.savedMenu.renameTitle.title'), entry.name);
    if (!name || name.trim() === '') return;
    await updateMutation.mutateAsync({ id: entry.id, patch: { name: name.trim() } });
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
        icon="empty-window"
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
    <template #footer>
      <Separator class="my-1" />
      <Alert v-if="error" variant="error" data-testid="kibana-saved-menu-error">
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <button
        type="button"
        :class="cn(rowVariants(), 'w-full text-primary')"
        data-testid="kibana-saved-save-current"
        @click="onSaveCurrent"
      >
        <span class="size-4 flex items-center justify-center shrink-0"
          ><CodiconIcon name="add" :size="13" /></span
        >
        {{ t('studio.kibana.saved.saveCurrent.label') }}
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
