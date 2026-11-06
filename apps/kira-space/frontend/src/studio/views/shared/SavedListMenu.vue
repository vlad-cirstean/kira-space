<script setup lang="ts" generic="Entry extends { id: string }">
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { PopoverContent } from '@theme/components/ui/popover';
import { Separator } from '@theme/components/ui/separator';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { ref } from 'vue';

// Popover shell for FilterHistoryMenu and ConsoleSavedMenu: a "Saved" list (pin, rename, delete,
// click to apply) with an optional click-to-apply "Recent" list. Callers own what each action does.
// `Entry` only needs an `id`; a "recent" entry may lack `pinned`, so pin state is duck-typed.
const props = defineProps<{
  /** Label for the saved section, e.g. "Saved" or "Saved queries". */
  title: string;
  saved: readonly Entry[];
  /** Omit entirely (undefined) to hide the "Recent" section — the console has no history list. */
  recent?: readonly Entry[];
  panelTestid: string;
  savedEntryTestid: string;
  recentEntryTestid?: string;
  emptySavedText: string;
  emptyRecentText?: string;
  /** Row text for first-letter type-ahead. */
  entryText: (entry: Entry) => string;
}>();

const emit = defineEmits<{
  apply: [entry: Entry];
  togglePin: [entry: Entry];
  delete: [entry: Entry];
}>();

// The list navigation claims Enter and the arrows; Space mirrors a click. The pin/delete buttons
// nested inside a saved row stay their own tab stops.
function onRowKeydown(e: KeyboardEvent, entry: Entry): void {
  if (e.key !== ' ' || e.target !== e.currentTarget) return;
  e.preventDefault();
  emit('apply', entry);
}

const savedEl = ref<HTMLElement | null>(null);
const savedActive = ref<string | null>(null);
const savedNav = useListNavigation<Entry>({
  container: savedEl,
  items: () => props.saved,
  key: (entry) => entry.id,
  activeKey: savedActive,
  onOpen: (entry) => emit('apply', entry),
  onSelect: (entry) => emit('apply', entry),
  typeahead: (entry) => props.entryText(entry),
});

const recentEl = ref<HTMLElement | null>(null);
const recentActive = ref<string | null>(null);
const recentNav = useListNavigation<Entry>({
  container: recentEl,
  items: () => props.recent ?? [],
  key: (entry) => entry.id,
  activeKey: recentActive,
  onOpen: (entry) => emit('apply', entry),
  onSelect: (entry) => emit('apply', entry),
  typeahead: (entry) => props.entryText(entry),
});

function isPinned(entry: Entry): boolean {
  return (
    typeof entry === 'object' &&
    entry !== null &&
    'pinned' in entry &&
    Boolean((entry as { pinned?: unknown }).pinned)
  );
}

defineSlots<{
  entry(props: { entry: Entry }): unknown;
  'entry-actions'?(props: { entry: Entry }): unknown;
  footer?(props: Record<string, never>): unknown;
}>();
</script>

<template>
  <PopoverContent align="start" class="w-80 gap-0 p-0" :data-testid="panelTestid">
    <div class="max-h-100 overflow-y-auto">
      <SectionHeading :label="title" />
      <InlineState v-if="saved.length === 0" kind="empty">{{ emptySavedText }}</InlineState>
      <div v-else ref="savedEl" role="listbox" :aria-label="title">
        <div
          v-for="entry in saved"
          :key="entry.id"
          :class="rowVariants()"
          :data-testid="savedEntryTestid"
          role="option"
          v-bind="savedNav.rowProps(entry.id)"
          :tabindex="savedNav.tabindexFor(entry.id)"
          @click="emit('apply', entry)"
          @keydown="onRowKeydown($event, entry)"
        >
          <Tooltip>
            <TooltipTrigger as-child>
              <button
                type="button"
                data-testid="saved-entry-pin"
                class="flex shrink-0 border-0 bg-transparent p-0"
                :class="isPinned(entry) ? 'text-warn' : 'text-subtle'"
                @click.stop="emit('togglePin', entry)"
              >
                <CodiconIcon :name="isPinned(entry) ? 'star-full' : 'star-empty'" :size="13" />
              </button>
            </TooltipTrigger>
            <TooltipContent>{{ t('studio.savedList.pin.tooltip') }}</TooltipContent>
          </Tooltip>
          <slot name="entry" :entry="entry" />
          <span class="flex shrink-0 gap-0.5">
            <slot name="entry-actions" :entry="entry" />
            <TooltipIconButton
              icon="trash"
              :label="t('common.action.delete.label')"
              @click.stop="emit('delete', entry)"
            />
          </span>
        </div>
      </div>

      <template v-if="recent">
        <Separator class="my-1" />
        <SectionHeading :label="t('studio.savedList.recent.label')" />
        <InlineState v-if="recent.length === 0" kind="empty">{{ emptyRecentText }}</InlineState>
        <div v-else ref="recentEl" role="listbox" :aria-label="t('studio.savedList.recent.label')">
          <div
            v-for="entry in recent"
            :key="entry.id"
            :class="rowVariants()"
            :data-testid="recentEntryTestid"
            role="option"
            v-bind="recentNav.rowProps(entry.id)"
            :tabindex="recentNav.tabindexFor(entry.id)"
            @click="emit('apply', entry)"
            @keydown="onRowKeydown($event, entry)"
          >
            <slot name="entry" :entry="entry" />
          </div>
        </div>
      </template>

      <slot name="footer" />
    </div>
  </PopoverContent>
</template>
