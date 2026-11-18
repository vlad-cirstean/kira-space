<script setup lang="ts">
import { shortSha } from '@kira/git-core';
/**
 * `docs/plans/P11.md` W12: the grouped dropdown `SearchBox.vue` renders inside its own `Popover`
 * (the same nesting `RefsPopover.vue` uses for `TagList.vue`/`StashList.vue`) — refs first
 * (local / remote / tag subsections, each labelled), then commits, each hit showing its kind and,
 * when it matched somewhere other than the subject or ref name, a field label. `role="listbox"`
 * with `aria-activedescendant` set on the *input* (`SearchBox.vue`'s own, per the ARIA combobox
 * pattern — this file never moves DOM focus onto a row), so option elements here carry no
 * `tabindex` of their own and clicking one never blurs the input; `searchResultsModel.ts` owns
 * every grouping/ordering/cap decision, this file only renders it.
 *
 * Keyboard (arrow-key nav, `Enter` to select, `Escape` to close) lives in `SearchBox.vue` — the
 * element that actually holds focus throughout — not here; this file only reflects
 * `highlightedId` back as `aria-selected` and forwards a click as `select`.
 *
 * P131 Part 2 §5.3: positioning, backdrop and Escape/click-outside handling all moved onto
 * `SearchBox.vue`'s own `Popover` — this file no longer owns any of that (its own outer box
 * classes, `w-105`/`max-h-90`/`overflow-y-auto`/`py-0.5`, moved onto `PopoverContent` there too),
 * rendering only the inner listbox content.
 */
import type { GitHostInfo } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatAgeCompact } from '@workbench/behaviour/format';
import Identifier from '@workbench/components/Identifier.vue';
import { computed } from 'vue';
import { BADGE_ICONS } from '../icons/index.ts';
import { SEARCH_LISTBOX_ID } from '../lib/searchListboxId.ts';
import type { SearchOption, SearchResultsModel } from '../lib/searchResultsModel.ts';
import { fieldLabel } from '../lib/searchResultsModel.ts';

const REF_ICON = {
  branch: BADGE_ICONS.localBranch,
  remoteBranch: BADGE_ICONS.remoteBranch,
  tag: BADGE_ICONS.tag,
} as const;

const props = defineProps<{
  model: SearchResultsModel;
  highlightedId: string | undefined;
  searching: boolean;
  tailStale: boolean;
  showBodySearchAffordance: boolean;
  host: GitHostInfo | null;
}>();

const emit = defineEmits<{
  (e: 'select', option: SearchOption): void;
  (e: 'hover', option: SearchOption): void;
  (e: 'runBodySearch'): void;
}>();

const isEmpty = computed(() => props.model.sections.length === 0);

/** `search-option`'s own gap/px are already exactly `rowVariants()`'s own
 *  `gap-1`/`px-1.5` (the default spacing scale directly).
 *  Only the vertical padding and the "active" (keyboard-highlighted, not a real `:hover`)
 *  background need adding —
 *  through `cn()` since `px-2` below replaces the variant's own `px-1.5` (different value,
 *  same property). */
function optionClass(option: SearchOption): string {
  return cn(
    rowVariants({ layout: 'tree', data: true, selected: option.id === props.highlightedId }),
    'pl-2',
  );
}
</script>

<template>
  <div data-testid="search-results">
    <!-- ARIA's listbox role only permits `option`/`group` children (`aria-required-children`) —
         the status line, section titles and options live inside this inner listbox div; the
         stale hint, footers and the body-search button are its *siblings*, not its children. -->
    <div :id="SEARCH_LISTBOX_ID" role="listbox" :aria-label="t('common.searchResults.aria')">
      <InlineState v-if="searching" kind="loading" data-testid="search-status">
        {{ t('common.searching.loading') }}
      </InlineState>

      <template v-for="section in model.sections" :key="section.title">
        <SectionHeading :label="section.title" :count="section.options.length" />

        <template v-for="option in section.options" :key="option.id">
          <div
            :id="option.id"
            role="option"
            :class="optionClass(option)"
            :aria-selected="option.id === highlightedId"
            tabindex="-1"
            @click="emit('select', option)"
            @mouseenter="emit('hover', option)"
            @keydown.enter="emit('select', option)"
          >
            <template v-if="option.kind === 'ref'">
              <CodiconIcon :name="REF_ICON[option.hit.ref.kind]" :size="13" />
              <span class="flex-1 min-w-0 truncate">{{ option.hit.ref.shortName }}</span>
              <Badge v-if="fieldLabel(option.hit.fields, host)">{{
                fieldLabel(option.hit.fields, host)
              }}</Badge>
            </template>
            <template v-else>
              <Identifier :value="option.hit.sha" :short="shortSha" class="text-muted-foreground" />
              <span class="flex-1 min-w-0 truncate">{{ option.hit.subject }}</span>
              <span :class="rowMeta">{{ option.hit.authorName }}</span>
              <span :class="rowMeta">{{ formatAgeCompact(option.hit.authorTime) }}</span>
              <Badge v-if="fieldLabel(option.hit.fields, host)">{{
                fieldLabel(option.hit.fields, host)
              }}</Badge>
            </template>
          </div>
        </template>
        <InlineState v-if="section.hiddenCount > 0" kind="empty">
          {{ t('git.search.hidden.hint', { count: section.hiddenCount }) }}
        </InlineState>
      </template>

      <InlineState v-if="isEmpty" kind="empty">{{ t('common.state.noMatches.empty') }}</InlineState>
    </div>

    <InlineState v-if="tailStale" kind="empty" data-testid="search-tail-stale">
      {{ t('git.search.tailStale.hint') }}
    </InlineState>
    <InlineState v-if="model.loadedFooter" kind="empty">{{ model.loadedFooter }}</InlineState>
    <InlineState v-if="model.tailFooter" kind="empty">{{ model.tailFooter }}</InlineState>
    <InlineState v-if="model.tailNotice" kind="empty" data-testid="search-tail-notice">
      {{ model.tailNotice }}
    </InlineState>
    <Button
      v-if="showBodySearchAffordance"
      variant="toolbar"
      size="kira"
      class="w-full justify-start"
      data-testid="search-body-button"
      @click="emit('runBodySearch')"
      ><CodiconIcon :name="ICON.search" :size="13" />
      {{ t('git.search.bodySearch.label') }}
    </Button>
  </div>
</template>
