<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { InputGroup } from '@theme/components/ui/input-group';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { rowVariants } from '@theme/lib/rowVariants';
import { autoClosePairsOnType, wrapSelectionOnType } from '@theme/wrapSelection';
import { useEventListener, useTimeoutFn } from '@vueuse/core';
import { loadMonaco, type MonacoModule } from '@workbench/editor/monaco';
import { computed, type HTMLAttributes, nextTick, ref, shallowRef, useId, watch } from 'vue';
import { isActiveDescendantKey, useActiveDescendant } from '../behaviour/activeDescendant';
import { revealElement } from '../behaviour/focus';
import { TIMING } from '../behaviour/timing';
import {
  type Completion,
  MAX_VISIBLE,
  rankCandidates,
  tokenAt as wordTokenAt,
} from '../editor/fieldCompletion';
import { monacoLanguageIdFor } from '../editor/monacoLanguages';
import { overlayOffsetAtPoint, paintOverlayHtml, paintRangesHtml } from '../editor/paintSpans';
import type { RangeHighlight } from '../editor/ranges';
import { shortcutFor } from '../shortcuts/keys';

// A free-text field with identifier/keyword completion (filter, header and URL inputs). The real
// <input>/<textarea> keeps DOM focus and its own keydown handling throughout; the popup is a reka
// Popover anchored to it that never takes focus (`@open-auto-focus.prevent`), with
// `useActiveDescendant` driving the current option. Attrs (data-testid, ...) land on the real
// element, not the wrapping box. Multiline grows with its content.
defineOptions({ inheritAttrs: false });

const MULTILINE_CLASS = 'h-auto min-h-control items-start py-1.5 leading-relaxed';
const props = withDefaults(
  defineProps<{
    /** A static list, or a function of the current token's context for a list that depends on where
     *  the caret is (`variableSupport` returns transforms after a `|`, names before one). */
    candidates:
      | readonly Completion[]
      | ((ctx: { text: string; from: number; word: string }) => readonly Completion[]);
    prefix?: string;
    /** Lights the prefix label when the underlying applied (not just typed) value is set. */
    prefixActive?: boolean;
    placeholder?: string;
    invalid?: boolean;
    /** A paint-only overlay behind the real input, coloured by Monaco's tokenizer. Never an editor
     *  instance: `locator.fill()` needs a real `<input>`/`<textarea>`. */
    language?: EditorLanguageId;
    /** Painted onto the overlay verbatim: ranges alone also create the overlay (`{{variable}}`). */
    rangeHighlights?: (doc: string) => readonly RangeHighlight[];
    /** Replaces the default word-run tokenizer. A `null` result clears the word and closes the
     *  popup, so a variable field only suggests while the caret is inside `{{…}}`. */
    tokenAt?: (text: string, caret: number) => { from: number; to: number; word: string } | null;
    /** Text-in lookup for the token under the pointer; no hover machinery exists when absent. */
    hoverAt?: (text: string, offset: number) => string[] | null;
    /** Opt-in auto-grow: the control becomes a `<textarea>` that never inserts a newline. */
    grow?: boolean;
    /** A resizable `rows`-tall `<textarea>`: Enter inserts a newline (unless a suggestion was
     *  arrowed onto) and Mod+Enter bubbles. */
    multiline?: boolean;
    rows?: number;
    /** Off for prose/shell bodies, where auto-closing `'` and `"` gets in the way. */
    autoClose?: boolean;
    /** Lists every candidate (capped at `MAX_VISIBLE`) as soon as `tokenAt` returns a non-null,
     *  empty word, e.g. right after typing `{` or `$`. */
    openOnEmptyToken?: boolean;
    /** Routed onto the InputGroup box, never the inner `<input>`. */
    class?: HTMLAttributes['class'];
  }>(),
  { candidates: () => [], autoClose: true, rows: 4 },
);

const text = defineModel<string>({ required: true });

const emit = defineEmits<{
  enter: [];
  escape: [];
  blur: [event: FocusEvent];
  /** Fired after a suggestion is inserted, so a caller can re-run its own parser/validator. */
  accept: [completion: Completion];
  /** The pasted text, plus its event so a host can `preventDefault()` and handle the paste. */
  pasteText: [text: string, event: ClipboardEvent];
}>();

const highlighted = computed(() => !!props.language && props.language !== 'plain');
const showOverlay = computed(() => highlighted.value || !!props.rangeHighlights);
// Ranges-only: the native control paints glyphs and caret in one layout; the overlay only tints
// behind it, so a layout difference can misplace a tint, never the caret.
const underlay = computed(() => !highlighted.value && !!props.rangeHighlights);
const overlayClass = computed(() =>
  props.grow || props.multiline
    ? 'block whitespace-pre-wrap wrap-anywhere leading-[inherit] [font-variant-ligatures:none]'
    : 'flex items-center whitespace-pre leading-[normal] [font-variant-ligatures:none]',
);
// Highlighted: text is transparent over the overlay so only its coloured glyphs show; the caret
// stays visible.
const fieldOverlayClass = computed(() => {
  if (!showOverlay.value) return 'text-fg';
  return underlay.value ? 'relative z-1 text-fg' : 'relative z-1 text-transparent caret-fg';
});
// CR and CRLF are line breaks in the control and the overlay, but plain spaces in `data-value`.
const displayText = computed(() => text.value.replace(/\r\n?/g, '\n'));

// Loaded lazily and shared app-wide; until it resolves the overlay paints plain text.
const monacoMod = shallowRef<MonacoModule | null>(null);
watch(
  highlighted,
  (h) => {
    if (h && !monacoMod.value) {
      void loadMonaco().then((mod) => {
        monacoMod.value = mod;
      });
    }
  },
  { immediate: true },
);

function escapePlain(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

const overlayHtml = ref('');
let paintGeneration = 0;
async function repaintOverlay(): Promise<void> {
  if (!showOverlay.value) {
    overlayHtml.value = '';
    return;
  }
  if (underlay.value) {
    overlayHtml.value = paintRangesHtml(
      displayText.value,
      props.rangeHighlights?.(displayText.value) ?? [],
    );
    return;
  }
  const mod = monacoMod.value;
  if (!mod) {
    overlayHtml.value = escapePlain(displayText.value);
    return;
  }
  const ranges = props.rangeHighlights ? props.rangeHighlights(displayText.value) : [];
  // Only the paint started last may win: a fast retype can outrun colorize()'s async grammar load.
  const generation = ++paintGeneration;
  const html = await paintOverlayHtml(
    mod,
    displayText.value,
    monacoLanguageIdFor(props.language ?? 'plain'),
    ranges,
  );
  if (generation === paintGeneration) overlayHtml.value = html;
}
watch(
  () => [text.value, props.language, props.rangeHighlights, monacoMod.value] as const,
  () => void repaintOverlay(),
  { immediate: true },
);

function onPaste(e: ClipboardEvent): void {
  const pasted = e.clipboardData?.getData('text') ?? '';
  if (pasted !== '') emit('pasteText', pasted, e);
}

const listId = `ac-${useId()}`;
const inputRef = ref<HTMLInputElement | HTMLTextAreaElement | null>(null);
const open = ref(false);
const wordStart = ref(0);
const currentWord = ref('');
// Ctrl+Space lists every candidate regardless of the token under the caret.
const forceAll = ref(false);
const tokenActive = ref(false);

const resolvedCandidates = computed<readonly Completion[]>(() =>
  typeof props.candidates === 'function'
    ? props.candidates({ text: text.value, from: wordStart.value, word: currentWord.value })
    : props.candidates,
);
const listAll = computed(
  () => forceAll.value || (props.openOnEmptyToken && tokenActive.value && currentWord.value === ''),
);
const filtered = computed(() =>
  listAll.value
    ? resolvedCandidates.value.slice(0, MAX_VISIBLE)
    : rankCandidates(resolvedCandidates.value, currentWord.value),
);
const wantsOpen = (): boolean =>
  (currentWord.value.length > 0 || (!!props.openOnEmptyToken && tokenActive.value)) &&
  filtered.value.length > 0;

// Enter accepts only after an explicit arrow onto an option, so "finish typing, Enter to submit"
// is never hijacked by a substring match. Tab accepts the current, possibly un-navigated, match.
const nav = useActiveDescendant<Completion>({
  items: () => filtered.value,
  optionId: (_, i) => `${listId}-${i}`,
  onSelect: accept,
  onActive: (_, i) => {
    const el = document.getElementById(`${listId}-${i}`);
    if (el) revealElement(el);
  },
  onEscape: () => {
    if (!open.value) return false;
    close();
    return true;
  },
  when: () => open.value && filtered.value.length > 0,
  initialIndex: 0,
  wrap: false,
});

function close(): void {
  open.value = false;
  forceAll.value = false;
}

function recompute(el: HTMLInputElement | HTMLTextAreaElement): void {
  const cursor = el.selectionStart ?? el.value.length;
  const token = (props.tokenAt ?? wordTokenAt)(el.value, cursor);
  wordStart.value = token?.from ?? cursor;
  currentWord.value = token?.word ?? '';
  tokenActive.value = token !== null;
  nav.reset();
}

function onInput(e: Event): void {
  const el = e.target as HTMLInputElement | HTMLTextAreaElement;
  text.value = el.value;
  forceAll.value = false;
  recompute(el);
  open.value = wantsOpen();
  closeHover();
}

// Cursor-only moves (click, arrow-left/right) still retarget the suggestion window; the keys
// onKeydown handles never move the text cursor and must not reset what it just set.
function onClick(e: Event): void {
  if (open.value) recompute(e.target as HTMLInputElement | HTMLTextAreaElement);
}
function onKeyup(e: KeyboardEvent): void {
  if (open.value && e.key !== 'Tab' && !isActiveDescendantKey(e.key)) {
    recompute(e.target as HTMLInputElement | HTMLTextAreaElement);
  }
}

function accept(completion: Completion): void {
  const el = inputRef.value;
  if (!el) return;
  const insertText = completion.insert ?? completion.label;
  const cursor = el.selectionStart ?? el.value.length;
  text.value = `${el.value.slice(0, wordStart.value)}${insertText}${el.value.slice(cursor)}`;
  emit('accept', completion);
  close();
  void nextTick().then(() => {
    const pos = wordStart.value + insertText.length - (completion.caretOffsetFromEnd ?? 0);
    el.setSelectionRange(pos, pos);
    el.focus();
  });
}

function onKeydown(e: KeyboardEvent): void {
  closeHover();
  // A bracket or quote typed over a selection wraps it; a collapsed caret auto-closes it. Either
  // edits the text, and onInput picks up the synthetic input event.
  const before = (e.target as HTMLInputElement | HTMLTextAreaElement).value;
  wrapSelectionOnType(e);
  if (props.autoClose) autoClosePairsOnType(e);
  if ((e.target as HTMLInputElement | HTMLTextAreaElement).value !== before) return;
  if (shortcutFor(e, ['editor.suggest']) !== null) {
    e.preventDefault();
    recompute(e.target as HTMLInputElement | HTMLTextAreaElement);
    forceAll.value = true;
    open.value = filtered.value.length > 0;
    return;
  }
  if (open.value && filtered.value.length > 0) {
    if (nav.onKeydown(e)) return;
    if (e.key === 'Tab') {
      e.preventDefault();
      accept(filtered.value[Math.max(nav.activeIndex.value, 0)] as Completion);
      return;
    }
  }
  if (e.key === 'Enter') {
    open.value = false;
    // Multiline: a plain Enter is a newline, Mod+Enter bubbles to the host (dialog save).
    if (props.multiline) return;
    // `grow` never inserts a newline: growth comes from soft wrapping alone.
    if (props.grow) e.preventDefault();
    emit('enter');
  } else if (e.key === 'Escape') emit('escape');
}

// The overlay never scrolls on its own; mirror the input's scroll so the coloured text stays under
// the invisible real text.
const overlayRootRef = ref<HTMLElement | null>(null);
function onInputScroll(e: Event): void {
  const el = e.target as HTMLInputElement | HTMLTextAreaElement;
  if (!overlayRootRef.value) return;
  overlayRootRef.value.scrollLeft = el.scrollLeft;
  overlayRootRef.value.scrollTop = el.scrollTop;
}

// Hover panel for the token under the pointer. The shared tooltip singleton hit-tests per element,
// never within one input as the pointer crosses onto a `{{ref}}`, so this field owns its own.
const hoverLines = ref<string[] | null>(null);
const hoverAnchor = ref({ x: 0, y: 0 });
// The same token yields the same lines, so comparing lines detects "a different token".
let lastHoverKey: string | null = null;

const { start: startHoverTimer, stop: stopHoverTimer } = useTimeoutFn(
  (x: number, y: number, lines: string[]) => {
    hoverAnchor.value = { x, y };
    hoverLines.value = lines;
  },
  TIMING.hoverInfo,
  { immediate: false },
);

function closeHover(): void {
  stopHoverTimer();
  hoverLines.value = null;
  lastHoverKey = null;
}

function onInputMouseMove(e: MouseEvent): void {
  if (!props.hoverAt) return;
  const overlay = overlayRootRef.value;
  const el = inputRef.value;
  if (!overlay || !el) return;
  // Hit-test the overlay (it paints the same text at the same coordinates) for an exact offset.
  // The input normally sits on top of it, so pointer events swap for this one synchronous call.
  el.style.pointerEvents = 'none';
  overlay.style.pointerEvents = 'auto';
  let offset: number | null;
  try {
    offset = overlayOffsetAtPoint(overlay, e.clientX, e.clientY);
  } finally {
    overlay.style.pointerEvents = 'none';
    el.style.pointerEvents = '';
  }
  const lines = offset === null ? null : props.hoverAt(displayText.value, offset);
  const key = lines ? JSON.stringify(lines) : null;
  if (key === lastHoverKey) return;
  lastHoverKey = key;
  stopHoverTimer();
  hoverLines.value = null;
  if (lines) startHoverTimer(e.clientX, el.getBoundingClientRect().top, lines);
}

function onBlur(e: FocusEvent): void {
  close();
  closeHover();
  emit('blur', e);
}

// A scroll or resize mid-selection is rare; a closed popup beats one stranded off its anchor.
function closeOnViewportChange(e?: Event): void {
  // The list scrolls itself (wheel, arrow onto an overflowed option); only an outer scroll strands it.
  if (e?.target instanceof Element && e.target.closest('.autocomplete-suggestions')) return;
  closeHover();
  if (open.value) close();
}
useEventListener(window, 'resize', closeOnViewportChange);
useEventListener(window, 'scroll', closeOnViewportChange, true);

// A click in the field is not "outside" the popup; anything else dismisses it.
function onInteractOutside(e: CustomEvent<{ originalEvent: Event }>): void {
  const target = e.detail.originalEvent.target;
  if (target instanceof Node && inputRef.value?.contains(target)) e.preventDefault();
  else close();
}

// ARIA roles ride on plain ul/li (specs locate `.autocomplete-suggestions li`); the options are
// focusable only so the role is valid, DOM focus stays in the input.
const LISTBOX_ATTRS = { role: 'listbox' } as const;
function optionAttrs(i: number) {
  return { role: 'option', 'aria-selected': i === nav.activeIndex.value, tabindex: -1 } as const;
}

const fieldAttrs = computed(
  () =>
    ({
      value: text.value,
      placeholder: props.placeholder,
      class: fieldOverlayClass.value,
      autocomplete: 'off',
      spellcheck: 'false',
      role: 'combobox',
      'aria-autocomplete': 'list',
      'aria-expanded': open.value,
      'aria-controls': listId,
      'aria-activedescendant':
        open.value && filtered.value[nav.activeIndex.value]
          ? `${listId}-${nav.activeIndex.value}`
          : undefined,
      onInput,
      onPaste,
      onClick,
      onKeyup,
      onKeydown,
      onBlur,
      onScroll: onInputScroll,
      onMousemove: onInputMouseMove,
      onMouseleave: closeHover,
    }) as const,
);
</script>

<template>
  <div class="contents">
    <Popover :open="open && filtered.length > 0">
      <InputGroup
        variant="kira"
        class="p-input autocomplete-field relative font-data"
        :class="[{ 'is-grow': grow }, multiline ? MULTILINE_CLASS : '', props.class]"
        :aria-invalid="invalid"
      >
        <span v-if="prefix" :class="prefixActive ? 'text-state-on' : 'text-muted-foreground'">{{
          prefix
        }}</span>
        <span
          class="input-wrap relative flex min-w-0 flex-1"
          :class="multiline ? 'items-stretch' : 'items-center'"
          :data-value="displayText"
        >
          <!-- Paint-only, aria-hidden, sized to sit exactly under the real input; `overlayHtml` is
           built entirely by `paintOverlayHtml` (HTML-escaped text in static class names). -->
          <div
            v-if="showOverlay"
            ref="overlayRootRef"
            class="highlight-overlay absolute inset-0 pointer-events-none overflow-hidden font-data text-kira-md bg-transparent"
            :class="[overlayClass, underlay ? 'is-underlay text-transparent' : '']"
            aria-hidden="true"
            v-html="overlayHtml"
          ></div>
          <textarea
            v-if="grow || multiline"
            ref="inputRef"
            :rows="multiline ? rows : 1"
            wrap="soft"
            class="min-w-0 flex-1 border-0 bg-transparent font-data text-kira-md outline-none placeholder:text-muted-foreground [font-variant-ligatures:none]"
            :class="multiline ? 'resize-y p-0 whitespace-pre-wrap wrap-anywhere' : ''"
            v-bind="{ ...$attrs, ...fieldAttrs }"
          />
          <input
            v-else
            ref="inputRef"
            class="min-w-0 flex-1 border-0 bg-transparent font-data text-kira-md outline-none placeholder:text-muted-foreground [font-variant-ligatures:none] [&::-webkit-inner-spin-button]:m-0 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:m-0 [&::-webkit-outer-spin-button]:appearance-none"
            v-bind="{ ...$attrs, ...fieldAttrs }"
          >
          <PopoverAnchor :reference="inputRef ?? undefined" />
        </span>
      </InputGroup>
      <PopoverContent
        side="bottom"
        align="start"
        :side-offset="4"
        flush
        class="autocomplete-suggestions z-(--kira-z-autocomplete) w-auto min-w-50 max-w-completion-max-w max-h-60 overflow-x-hidden overflow-y-auto font-data"
        @open-auto-focus.prevent
        @close-auto-focus.prevent
        @interact-outside="onInteractOutside"
        @mousedown.prevent
      >
        <ul :id="listId" v-bind="LISTBOX_ATTRS" class="m-0 list-none p-0.5">
          <li
            v-for="(c, i) in filtered"
            :id="`${listId}-${i}`"
            :key="c.label"
            v-bind="optionAttrs(i)"
            :class="rowVariants({ layout: 'menu', selected: i === nav.activeIndex.value })"
            @mouseenter="nav.activeIndex.value = i"
            @mousedown.prevent="accept(c)"
          >
            <CodiconIcon
              v-if="c.icon"
              :name="c.icon"
              :size="13"
              class="shrink-0 text-muted-foreground"
            />
            <span class="truncate">{{ c.label }}</span>
            <span
              v-if="c.detail"
              class="ml-auto pl-1.5 text-muted-foreground text-kira-sm shrink-0"
              >{{
                c.detail
              }}</span
            >
          </li>
        </ul>
      </PopoverContent>
    </Popover>
    <Popover :open="hoverLines !== null">
      <PopoverAnchor as-child>
        <span
          class="fixed size-0"
          aria-hidden="true"
          :style="{ left: `${hoverAnchor.x}px`, top: `${hoverAnchor.y}px` }"
        />
      </PopoverAnchor>
      <!-- Above the field: below it the popper wrapper (which has pointer events) covers the
           pointer, the input gets mouseleave and the panel closes itself. -->
      <PopoverContent
        side="top"
        align="start"
        :side-offset="4"
        flush
        class="var-hover-panel pointer-events-none z-(--kira-z-autocomplete) w-auto max-w-96 font-data"
        data-testid="autocomplete-hover"
        @open-auto-focus.prevent
        @close-auto-focus.prevent
        @interact-outside.prevent
      >
        <div class="px-1.5 py-1">
          <div
            v-for="(line, i) in hoverLines"
            :key="i"
            class="hover-line whitespace-pre-wrap wrap-anywhere"
            :class="{ 'mt-0.5 text-muted-foreground': i > 0 }"
            >{{
              line
            }}</div
          >
        </div>
      </PopoverContent>
    </Popover>
  </div>
</template>
