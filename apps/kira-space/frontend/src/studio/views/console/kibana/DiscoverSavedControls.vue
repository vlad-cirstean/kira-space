<script setup lang="ts">
import type { KibanaDiscoverBody } from '@shared/domain/kibana';
import type { SavedDiscoverQuery } from '@shared/domain/queries';
import { useMutation, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Kbd } from '@theme/components/ui/kbd';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { errorText, toCodedError } from '@workbench/bridge/codedError';
import MenuItems from '@workbench/components/MenuItems.vue';
import TextPromptDialog from '@workbench/prompt/TextPromptDialog.vue';
import { useTextPrompt } from '@workbench/prompt/useTextPrompt';
import { formatShortcut } from '@workbench/shortcuts/keys';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { control } from '../../../bridge/control';
import { guardedSavedWrite } from '../../../state/guardedSavedWrite';
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
  mutationFn: (args: { id: string; loaded: KibanaDiscoverBody }) => {
    const body = props.d.body.value;
    if (!body) throw new Error(t('studio.kibana.saved.noView.error'));
    return guardedSavedWrite((guarded) =>
      control.queriesUpdateDiscover(args.id, body, guarded ? args.loaded : undefined),
    );
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
      const saved = await updateMutation.mutateAsync({ id: link.id, loaded: link.body });
      // Undefined: the user declined to overwrite another window's change.
      if (saved) props.d.link(saved);
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

const moreItems = computed<MenuItem[]>(() => [
  {
    type: 'item',
    id: 'save-as',
    testId: 'kibana-save-as',
    label: t('studio.kibana.saved.saveAs.label'),
    icon: 'save-as',
    disabled: !props.d.body.value || pending(),
    run: onSaveAs,
  },
  {
    type: 'item',
    id: 'revert',
    testId: 'kibana-revert',
    label: t('studio.kibana.saved.revert.label'),
    icon: 'discard',
    disabled: !props.d.state.value.saved || !props.d.dirty.value,
    run: onRevert,
  },
  ...(props.d.state.value.saved
    ? [
        {
          type: 'item' as const,
          id: 'detach',
          testId: 'kibana-detach',
          label: t('studio.kibana.saved.detach.label'),
          icon: 'debug-disconnect',
          run: () => props.d.unlink(),
        },
      ]
    : []),
]);

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
          <MenuItems :items="moreItems" />
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    <TextTooltip :text="t('studio.console.savedMenu.title')">
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
    </TextTooltip>
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
