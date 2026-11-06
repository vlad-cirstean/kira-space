<script setup lang="ts">
import type { SavedDiscoverQuery } from '@shared/domain/queries';
import { useMutation, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Kbd } from '@theme/components/ui/kbd';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { errorText, toCodedError } from '@workbench/bridge/codedError';
import TextPromptDialog from '@workbench/prompt/TextPromptDialog.vue';
import { useTextPrompt } from '@workbench/prompt/useTextPrompt';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { ref, useTemplateRef, watch } from 'vue';
import { control } from '../../../bridge/control';
import DiscoverSavedMenu from './DiscoverSavedMenu.vue';
import type { useDiscover } from './useDiscover';

const props = defineProps<{ d: ReturnType<typeof useDiscover>; connectionId: string }>();
const emit = defineEmits<{ run: []; error: [message: string | null] }>();

const queryClient = useQueryClient();
const invalidate = () => queryClient.invalidateQueries({ queryKey: ['kibanaSaved'] });

const {
  prompt: textPrompt,
  open: promptText,
  submit: submitPrompt,
  cancel: cancelPrompt,
} = useTextPrompt();

const failure = ref<string | null>(null);
watch(failure, (message) => emit('error', message));
defineExpose({ save: onSave });

const saveMutation = useMutation({
  mutationFn: (args: { name: string }) => {
    const body = props.d.body.value;
    if (!body) throw new Error(t('studio.kibana.saved.noView.error'));
    return control.queriesSaveDiscover({
      connectionId: props.connectionId,
      name: args.name,
      body,
      pinned: false,
    });
  },
  onSuccess: invalidate,
});
const updateMutation = useMutation({
  mutationFn: (args: { id: string }) => {
    const body = props.d.body.value;
    if (!body) throw new Error(t('studio.kibana.saved.noView.error'));
    return control.queriesUpdateDiscover(args.id, body);
  },
  onSuccess: invalidate,
});
const pending = () => saveMutation.isPending.value || updateMutation.isPending.value;

async function saveAs(initial?: string): Promise<void> {
  const name = await promptText(
    t('studio.kibana.saved.nameTitle.title'),
    initial ?? props.d.state.value.saved?.name ?? props.d.state.value.dataViewName ?? '',
  );
  if (!name || name.trim() === '') return;
  const entry = await saveMutation.mutateAsync({ name: name.trim() });
  props.d.link(entry);
}

async function attempt(action: () => Promise<void>): Promise<void> {
  failure.value = null;
  try {
    await action();
  } catch (err) {
    failure.value = errorText(err);
  }
}

function onSaveAs(): Promise<void> {
  return attempt(() => saveAs());
}

function onSave(): Promise<void> {
  if (!props.d.body.value || pending()) return Promise.resolve();
  const link = props.d.state.value.saved;
  if (!link) return onSaveAs();
  return attempt(async () => {
    try {
      props.d.link(await updateMutation.mutateAsync({ id: link.id }));
    } catch (err) {
      if (toCodedError(err).code !== 'E_NOT_FOUND') throw err;
      // Deleted elsewhere: the tab keeps its state and offers the old name again.
      props.d.unlink();
      await saveAs(link.name);
    }
  });
}

function onRevert(): void {
  const link = props.d.state.value.saved;
  if (!link) return;
  failure.value = null;
  if (props.d.applySaved(link)) emit('run');
}

function onApply(entry: SavedDiscoverQuery): void {
  failure.value = null;
  if (props.d.applySaved(entry)) emit('run');
}

const menuOpen = ref(false);
const menuTrigger = useTemplateRef<{ $el: HTMLElement }>('menuTrigger');
const saveChord = formatShortcut('form.save');
</script>

<template>
  <div class="ml-auto flex items-center gap-1" data-testid="kibana-saved-controls">
    <span
      v-if="d.state.value.saved"
      class="flex min-w-0 items-center gap-1 text-kira-md"
      data-testid="kibana-saved-name"
    >
      <CodiconIcon name="bookmark" :size="12" class="shrink-0 text-muted-foreground" />
      <span class="truncate">{{ d.state.value.saved.name }}</span>
      <StatusDot
        v-if="d.dirty.value"
        tone="warn"
        :label="t('common.state.unsavedChanges.status')"
        data-testid="kibana-saved-dirty"
      />
    </span>
    <div class="flex items-center">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button
            variant="toolbar"
            size="kira"
            :disabled="!d.body.value"
            :pending="pending()"
            data-testid="kibana-save"
            @click="onSave"
          >
            <CodiconIcon name="save" :size="13" />
            {{ t('common.action.save.label') }}
          </Button>
        </TooltipTrigger>
        <TooltipContent
          >{{ t('studio.kibana.saved.save.tooltip') }}<Kbd>{{ saveChord }}</Kbd></TooltipContent
        >
      </Tooltip>
      <DropdownMenu :modal="false">
        <DropdownMenuTrigger as-child>
          <Button
            variant="toolbar"
            size="kira"
            :aria-label="t('studio.kibana.saved.more.aria')"
            data-testid="kibana-save-more"
          >
            <CodiconIcon name="chevron-down" :size="12" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            :disabled="!d.body.value || pending()"
            data-testid="kibana-save-as"
            @select="onSaveAs"
          >
            {{ t('studio.kibana.saved.saveAs.label') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            :disabled="!d.state.value.saved || !d.dirty.value"
            data-testid="kibana-revert"
            @select="onRevert"
          >
            {{ t('studio.kibana.saved.revert.label') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            v-if="d.state.value.saved"
            data-testid="kibana-detach"
            @select="d.unlink()"
          >
            {{ t('studio.kibana.saved.detach.label') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    <Tooltip>
      <TooltipTrigger as-child>
        <Button
          ref="menuTrigger"
          variant="toolbar"
          size="kira"
          data-testid="kibana-saved-toggle"
          @click="menuOpen = !menuOpen"
        >
          <CodiconIcon name="bookmark" :size="13" />
          {{ t('studio.console.savedMenu.title') }}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{{ t('studio.console.savedMenu.title') }}</TooltipContent>
    </Tooltip>
    <Popover :open="menuOpen" @update:open="(v) => (menuOpen = v)">
      <PopoverAnchor :reference="(menuTrigger?.$el as HTMLElement) ?? undefined" class="hidden" />
      <DiscoverSavedMenu
        v-if="menuOpen"
        :connection-id="connectionId"
        :linked-id="d.state.value.saved?.id ?? null"
        :dirty="d.dirty.value"
        @apply="onApply"
        @save-current="onSaveAs"
        @close="menuOpen = false"
      />
    </Popover>
    <TextPromptDialog
      v-if="textPrompt"
      :title="textPrompt.title"
      :model-value="textPrompt.value"
      @update:model-value="(v) => textPrompt && (textPrompt.value = v)"
      @submit="submitPrompt"
      @close="cancelPrompt"
    />
  </div>
</template>
