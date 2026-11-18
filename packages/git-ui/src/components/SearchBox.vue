<script setup lang="ts">
/**
 * `docs/plans/P11.md` W11/W12: §6.2's search box. One text input, three toggle buttons
 * (case-sensitive / whole-word / regex) as `aria-pressed` icon buttons with
 * icon-button styling, a Commits/Refs/Both scope select living *inside* the box (OQ6: §6.2
 * gave the toolbar one slot and §6.3's 600px breakpoint had no room for a second control — G-UX
 * D9/item 9 later moved this whole component out of the toolbar into its own row below it, but
 * the box's own internal anatomy is unchanged), the inline `n of N` match count, an inline regex
 * error wired to the input via `aria-describedby` (probe 4) — and, nested inside this same root
 * the way `RefsPopover.vue` nests `TagList.vue`/`StashList.vue` inside its own open panel,
 * `SearchResults.vue`'s grouped dropdown.
 *
 * **Why the dropdown lives here, not as an `App.vue`-positioned sibling.** `SearchResults.vue`
 * follows the ARIA combobox pattern (`role="listbox"` + `aria-activedescendant`), which requires
 * DOM focus to stay on the input the whole time an option is "active" — arrow keys move a virtual
 * cursor, never real focus, so typing and arrowing interleave freely and a mouse click on a row
 * never blurs the box. That means the arrow-key/Enter/Escape handling for the dropdown has to
 * live wherever the input's own `keydown` fires, which is this file; `SearchResults.vue` itself
 * stays a pure renderer over `searchResultsModel.ts`'s fold plus the `highlightedId` this file
 * hands it, emitting only a plain `select`/`hover`/`runBodySearch` this file forwards or acts on.
 *
 * **Reconciling `Enter` with "next match" (hard part 8, judgment call 6).** Two different things
 * both want `Enter`: `§7.8`'s "step through commit matches" and the listbox's own "accept the
 * highlighted option". `descendant.activeIndex` is the tie-break: `-1` (nothing arrowed to yet) means
 * `Enter`/`Shift+Enter` step `SearchState.next()`/`previous()` exactly as `§7.8` describes commit
 * matches only, never refs; the moment an arrow key has moved that cursor onto some option,
 * `Enter` accepts *that* option instead (ref or commit alike) and `Shift+Enter` reverts to no-op,
 * since the dropdown has no symmetric "previous option" gesture of its own to give it.
 *
 * **`Escape`'s two-stage order (spec edit 6, OQ5).** OQ5 says closing the dropdown must not erase
 * the in-place highlights — a user closing it to look at the graph is still mid-search. So a
 * first `Escape` while the dropdown is open only dismisses it (`#dropdownDismissed`), leaving the
 * query and every derived highlight untouched; a second `Escape` (or the first, when the dropdown
 * was already closed or never open) clears the query via `SearchState.clear()` and asks the
 * parent to move focus back to the grid (`focusGrid`). An empty box with no dropdown open lets
 * the key fall through undisturbed to `App.vue`'s own Esc chain (diff view, then detail pane) —
 * together this *is* spec edit 6's "an open menu, then the search results dropdown, then the diff
 * view, then the detail pane" without `App.vue`'s own handler needing to know anything about
 * search at all.
 *
 * **`/` and `Ctrl/Cmd+F`** no longer live here (G-UX D9, item 9): the search row this component
 * mounts inside is now conditionally rendered (`App.vue`'s `searchOpen`), so a listener mounted
 * for "this component's own whole lifetime" would only ever fire while the row is already open —
 * exactly backwards for a shortcut whose whole job is opening it. `App.vue` owns that listener
 * now, alongside the toggle-closed gesture (`Ctrl/Cmd+F` a second time) and the host's
 * `Ctrl+Alt+F` keybinding, none of which this component needs to know about.
 *
 * P131 Part 2 §5.3: the dropdown and the regex error now share one non-modal shadcn `Popover`
 * (mutually exclusive — the error means `compiled.kind` failed, the dropdown needs it `'ok'`),
 * anchored to this row via `PopoverAnchor`. The row's own two-stage Escape and the input's real
 * DOM focus stay exactly as this file's own doc comment above describes — the popover's own
 * dismissal (Escape/outside-click) is wired to defer to them rather than fight them (`@escape-
 * key-down`/`@interact-outside`/`@focus-outside` below), the ARIA combobox contract's real focus
 * requirement unchanged.
 */
import type { SearchScope } from '@kira/git-core';
import InlineState from '@theme/components/InlineState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { useActiveDescendant } from '@workbench/behaviour/activeDescendant';
import SearchOptionToggles from '@workbench/components/SearchOptionToggles.vue';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { SEARCH_LISTBOX_ID } from '../lib/searchListboxId.ts';
import type { SearchOption } from '../lib/searchResultsModel.ts';
import { buildSearchResultsModel } from '../lib/searchResultsModel.ts';
import { useFieldFocus } from '../lib/useFieldFocus.ts';
import type { SearchState } from '../state/search.ts';
import { MIN_TAIL_QUERY_LENGTH } from '../state/search.ts';
import SearchResults from './SearchResults.vue';

const props = defineProps<{ search: SearchState }>();
const emit = defineEmits<{
  (e: 'focusGrid'): void;
  (e: 'select', option: SearchOption): void;
  /** The row's own close (X) button — `App.vue` owns `closeSearch()` (close, clear the query, and
   *  return focus to the grid), the same "emit, don't own" shape this component already follows
   *  for `focusGrid`. */
  (e: 'close'): void;
}>();

const rootEl = ref<HTMLElement | null>(null);
const searchInputEl = useTemplateRef<{ $el: Element }>('searchInputEl');

const ERROR_ID = 'git-search-error';

/** The inline `n of N` indicator mirrors exactly what `Enter`/`Shift+Enter` step through —
 *  commit matches — so it is hidden entirely in `Refs` scope, where there is
 *  nothing to page through at all; `SearchResults.vue`'s own group counts are what show a
 *  ref-hit count, in both `Refs` and `Both` scope alike. */
const countLabel = computed(() => {
  if (props.search.compiled.value.kind !== 'ok') return '';
  if (props.search.scope.value === 'refs') return '';
  const { n, exact } = props.search.matchCount.value;
  if (n === 0) return t('common.state.noMatches.empty');
  const position = props.search.activeIndex.value >= 0 ? props.search.activeIndex.value + 1 : 1;
  return t('common.matchPosition.status', { position, total: exact ? String(n) : `${n}+` });
});

// ---------------------------------------------------------------------------------------
// The dropdown: visible whenever the query compiles to something real, dismissible without
// touching the query, reopened by the next keystroke.
// ---------------------------------------------------------------------------------------
const dropdownDismissed = ref(false);

const resultsModel = computed(() =>
  buildSearchResultsModel({
    scope: props.search.scope.value,
    refHits: props.search.refHits.value,
    commitHits: props.search.commitHits.value,
    loaded: props.search.loaded.value,
    loadedRowCount: props.search.loadedRowCount.value,
    tail: props.search.tail.value,
    tailError: props.search.tailError.value,
  }),
);

const dropdownVisible = computed(
  () => props.search.compiled.value.kind === 'ok' && !dropdownDismissed.value,
);

// P131 Part 2 §5.3: one non-modal Popover shows whichever of the two applies -- the dropdown or
// the regex error -- they are mutually exclusive (results need compiled.kind === 'ok', the error
// means it failed).
const popoverOpen = computed(() => !!props.search.error.value || dropdownVisible.value);

function onPopoverOpenChange(value: boolean): void {
  if (value) return;
  // The error is passive and clears only when the query changes (not on dismissal) -- ignore a
  // reka-driven close while it is showing, matching the old hand-positioned popup's own
  // "no Escape/click-outside handling of its own" for this element.
  if (props.search.error.value) return;
  dropdownDismissed.value = true;
}

// A click/focus move inside this component's own row (the input, the option toggles, the scope
// select, the close button) must not count as "outside" the popover -- it is the anchor itself,
// portaled content included would otherwise get a doubled close.
function ignoreOwnRow(e: CustomEvent<{ originalEvent: PointerEvent | FocusEvent }>): void {
  const target = e.detail.originalEvent.target as Node | null;
  if (target && rootEl.value?.contains(target)) e.preventDefault();
}

// Focus stays in the input; arrows move `aria-activedescendant`. Enter selects only once an arrow
// moved the cursor, otherwise it steps commit matches (judgment call 6 above).
const descendant = useActiveDescendant<SearchOption>({
  items: () => resultsModel.value.flatOptions,
  optionId: (option) => option.id,
  onSelect: (option) => selectOption(option),
  when: () => dropdownVisible.value,
});
const highlightedOption = computed<SearchOption | undefined>(
  () => resultsModel.value.flatOptions[descendant.activeIndex.value],
);

/** OQ1's on-demand override, shown only when that is the *actual* reason the tail has not run —
 *  never for a too-short query or `Refs` scope, where offering it would be a lie about what it
 *  does. */
const showBodySearchAffordance = computed(
  () =>
    props.search.compiled.value.kind === 'ok' &&
    props.search.scope.value !== 'refs' &&
    props.search.text.value.length >= MIN_TAIL_QUERY_LENGTH &&
    props.search.tail.value === undefined &&
    props.search.tailSkippedByExhaustion.value,
);

// Typing (or a toggle/scope change producing a new query) always reopens a dismissed dropdown
// and drops whatever option an earlier query's arrow-key nav had reached — that cursor no longer
// refers to anything meaningful once the option list has been rebuilt.
watch(
  () => props.search.compiled.value,
  () => {
    dropdownDismissed.value = false;
    descendant.reset();
  },
);

function onHover(option: SearchOption): void {
  descendant.activeIndex.value = resultsModel.value.flatOptions.indexOf(option);
  descendant.navigated.value = true;
}

function selectOption(option: SearchOption): void {
  dropdownDismissed.value = true;
  emit('select', option);
}

/** `RegExp`'s message embeds the whole compiled source (`/ab(/: Unterminated group`), which
 *  changes on every keystroke and would re-announce the alert each time. Only the error kind stays. */
const errorText = computed(() => props.search.error.value?.replace(/^\/[\s\S]*\/[a-z]*: /, ''));

function onKeydown(event: KeyboardEvent): void {
  // An IME composition's Enter/Escape commit or cancel the composition, not the search.
  if (event.isComposing || event.keyCode === 229) return;
  if (descendant.onKeydown(event)) return;
  if (event.key === 'Enter') {
    event.preventDefault();
    if (event.shiftKey) props.search.previous();
    else props.search.next();
    return;
  }
  if (event.key === 'Escape') {
    if (dropdownVisible.value) {
      event.preventDefault();
      event.stopPropagation();
      dropdownDismissed.value = true;
      return;
    }
    if (props.search.text.value === '') return; // fall through to App.vue's own Esc chain
    event.preventDefault();
    event.stopPropagation();
    props.search.clear();
    emit('focusGrid');
  }
}

const SCOPE_ITEMS = computed(() => [
  { value: 'both', label: t('git.search.scope.both.option'), testid: 'search-scope-both' },
  { value: 'commits', label: t('common.module.commits.label'), testid: 'search-scope-commits' },
  { value: 'refs', label: t('git.common.refs.label'), testid: 'search-scope-refs' },
]);

function onScopeChange(value: string): void {
  props.search.scope.value = value as SearchScope;
}

const focusInput = useFieldFocus(searchInputEl);
defineExpose({ focusInput });
</script>

<template>
  <Popover :open="popoverOpen" @update:open="onPopoverOpenChange">
    <PopoverAnchor as-child>
      <div ref="rootEl" class="relative flex-1 min-w-0">
        <div class="flex items-center gap-1">
          <div class="min-w-0 flex-1" data-testid="search-input">
            <SearchField
              ref="searchInputEl"
              v-model="search.text.value"
              class="font-data"
              role="combobox"
              tabindex="0"
              aria-haspopup="listbox"
              :aria-expanded="dropdownVisible && resultsModel.sections.length > 0"
              :aria-controls="dropdownVisible ? SEARCH_LISTBOX_ID : undefined"
              :aria-activedescendant="descendant.activeId.value"
              :aria-describedby="search.error.value ? ERROR_ID : undefined"
              :aria-invalid="!!search.error.value"
              @keydown="onKeydown"
            />
          </div>
          <section class="flex gap-0.5" :aria-label="t('git.search.options.aria')">
            <SearchOptionToggles
              v-model:match-case="search.caseSensitive.value"
              v-model:whole-word="search.wholeWord.value"
              v-model:regex="search.regex.value"
              testid-prefix="search-toggle-"
              match-case-testid="search-toggle-case"
            />
          </section>
          <SecondaryTabs
            variant="segmented"
            :aria-label="t('git.search.scope.aria')"
            data-testid="search-scope"
            :model-value="search.scope.value"
            :items="SCOPE_ITEMS"
            @update:model-value="onScopeChange"
          />
          <span
            v-if="countLabel"
            class="px-0.5 text-muted-foreground text-kira-sm whitespace-nowrap tabular-nums"
            data-testid="search-count"
            >{{
              countLabel
            }}</span
          >
          <TooltipIconButton
            icon="close"
            :label="t('common.action.closeSearch.label')"
            data-testid="search-close-button"
            @click="emit('close')"
          />
        </div>
      </div>
    </PopoverAnchor>

    <PopoverContent
      align="start"
      :side-offset="2"
      flush
      class="w-120"
      @open-auto-focus.prevent
      @close-auto-focus.prevent
      @escape-key-down="(e) => e.preventDefault()"
      @interact-outside="ignoreOwnRow"
      @focus-outside="ignoreOwnRow"
    >
      <InlineState v-if="search.error.value" :id="ERROR_ID" kind="error" data-testid="search-error">
        {{ errorText }}
      </InlineState>
      <SearchResults
        v-else
        :model="resultsModel"
        :highlighted-id="highlightedOption?.id"
        :searching="search.searching.value"
        :tail-stale="search.tailStale.value"
        :show-body-search-affordance="showBodySearchAffordance"
        :host="search.host.value"
        @select="selectOption"
        @hover="onHover"
        @run-body-search="search.runBodySearch()"
      />
    </PopoverContent>
  </Popover>
</template>
