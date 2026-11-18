<script setup lang="ts" generic="Entry extends { id: string }">
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { PopoverContent, PopoverFooter, PopoverList } from '@theme/components/ui/popover';
import { Separator } from '@theme/components/ui/separator';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { type ComponentPublicInstance, computed, ref, useTemplateRef } from 'vue';

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

// List navigation needs the DOM element behind the PopoverList component.
const savedList = useTemplateRef<ComponentPublicInstance>('savedList');
const savedEl = computed<HTMLElement | null>(() => savedList.value?.$el ?? null);
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

const recentList = useTemplateRef<ComponentPublicInstance>('recentList');
const recentEl = computed<HTMLElement | null>(() => recentList.value?.$el ?? null);
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
  /** Between the lists and the footer: alerts, and dialogs that must stay inside the popover. */
  below?(props: Record<string, never>): unknown;
  /** Action row: `toolbar` `kira` Buttons with a leading icon. */
  footer?(props: Record<string, never>): unknown;
}>();
</script>

<template>
  <PopoverContent align="start" flush class="w-80" :data-testid="panelTestid">
    <div class="max-h-100 overflow-y-auto">
      <SectionHeading :label="title" />
      <InlineState v-if="saved.length === 0" kind="empty">{{ emptySavedText }}</InlineState>
      <PopoverList v-else ref="savedList" role="listbox" :aria-label="title">
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
          <TextTooltip :text="t('studio.savedList.pin.tooltip')">
            <button
              type="button"
              data-testid="saved-entry-pin"
              class="flex shrink-0 border-0 bg-transparent p-0"
              :class="isPinned(entry) ? 'text-warn' : 'text-subtle'"
              @click.stop="emit('togglePin', entry)"
            >
              <CodiconIcon :name="isPinned(entry) ? 'star-full' : 'star-empty'" :size="13" />
            </button>
          </TextTooltip>
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
      </PopoverList>

      <template v-if="recent">
        <Separator class="my-1" />
        <SectionHeading :label="t('studio.savedList.recent.label')" />
        <InlineState v-if="recent.length === 0" kind="empty">{{ emptyRecentText }}</InlineState>
        <PopoverList
          v-else
          ref="recentList"
          role="listbox"
          :aria-label="t('studio.savedList.recent.label')"
        >
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
        </PopoverList>
      </template>

      <slot name="below" />
      <PopoverFooter><slot name="footer" /></PopoverFooter>
    </div>
  </PopoverContent>
</template>
