<script
  setup
  lang="ts"
  generic="T extends { name: string; value: string; enabled?: boolean; description?: string }"
>
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { InputGroup, InputGroupTextarea } from '@theme/components/ui/input-group';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useEventListener } from '@vueuse/core';
import { revealElement } from '@workbench/behaviour/focus';
import { matchesQuery } from '@workbench/behaviour/match';
import AutocompleteField from '@workbench/components/AutocompleteField.vue';
import type { Completion } from '@workbench/editor/fieldCompletion';
import { templateToken, wholeFieldToken } from '@workbench/editor/fieldCompletion';
import { computed, nextTick, ref, watch } from 'vue';
import type { VariableSupport } from '../../../api/state/variableCompletion';

// The one row table behind Params, Headers, urlencoded and form-data —
// RequestHeadersTable.vue and QueryParamsTable.vue were the same file twice minus a checkbox
// column. Each wrapper keeps its own write semantics (headers/urlencoded write their array
// directly; Params writes back through the URL) and supplies only `rows` (the real,
// non-trailing rows) plus a blank-row factory; this component owns the trailing-blank-row UX, the
// optional checkbox column, and the always-disabled-on-the-trailing-row remove button.
const props = withDefaults(
  defineProps<{
    /** A fresh blank row — each wrapper's own row shape (headers default `enabled: true`, params
     *  carry no `enabled` at all), so this can't be derived generically. */
    blankRow: () => T;
    /** Renders the checkbox column. Off for Params (P2 D6/§8 OQ-1: no disabled query params). */
    showEnabled?: boolean;
    namePlaceholder?: string;
    valuePlaceholder?: string;
    /** `${testidPrefix}-row` / `-name` / `-value` / `-enabled` / `-remove` — must reproduce the
     *  exact testids the pre-extraction tables used (C6's own guard). */
    testidPrefix: string;
    /** The outer container's own testid (e.g. `http-headers-table`) — kept at the wrapper's
     *  discretion since the two existing tables pluralize their prefix differently. */
    containerTestid?: string;
    /** When present, the default value cell renders as an AutocompleteField wired for
     *  `{{variable}}` colouring/hover/completion instead of a plain TextField — absent for any
     *  caller that has not been wired (form-data's own slot wires this itself, since it
     *  overrides the `value` slot entirely). */
    valueVariableSupport?: VariableSupport;
    /** When present, the name cell renders as an AutocompleteField over this
     *  list, using `wholeFieldToken` (the default word-run tokenizer has no `-`, so typing
     *  `Content-T` would tokenize as just `T` and accepting a suggestion would produce
     *  `Content-Content-Type`). Absent for every caller but the headers table. */
    nameCandidates?: readonly Completion[];
    /** The value cell's own vocabulary as a function of this row's name — composed with,
     *  not replacing, valueVariableSupport.candidates (a header value is very often
     *  `Bearer {{token}}`, which needs both lists live at once). Absent for every caller but the
     *  headers table. */
    valueCandidatesFor?: (rowName: string) => readonly Completion[];
    /** P16 D13: the toolbar's own filter box text, forwarded down — absent/empty shows every row.
     *  A plain case-insensitive substring test over name-or-value (both are user-authored request
     *  content, never a secret's plaintext — §5). */
    filterQuery?: string;
    /** P22b D6/D7: renders the description column — off by default (OQ-1: a fourth
     *  AutocompleteField-width column in an already-tight row is unusable for the majority of rows
     *  that have no description). Driven by a per-tab toggle at the call site. */
    showDescriptions?: boolean;
  }>(),
  {
    showEnabled: false,
    namePlaceholder: 'key',
    valuePlaceholder: 'value',
    showDescriptions: false,
  },
);

const rows = defineModel<T[]>('rows', { required: true });

// P28 D10: a frozen module-level constant, not an inline `[]` in the template — a fresh array
// literal per render would change `candidates`' identity on every patch.
const EMPTY_CANDIDATES: readonly Completion[] = Object.freeze([]);

// A CSS grid, not independent flex items — the finding was that adjacent rows'
// columns never lined up (each field carried its own `flex` value, so a secret/file row with an
// extra affordance shifted its neighbours). `showEnabled`/`showDescriptions` are fixed per table
// instance (never vary row to row), so the column count below is a property of the *table*, not
// the row — unlike the trailing slot (FormDataTable's kind/file controls), which genuinely does
// vary row to row and is wrapped in its own single grid cell below for exactly that reason.
const gridTemplateColumns = computed(() => {
  const checkbox = props.showEnabled ? 'auto ' : '';
  const description = props.showDescriptions ? '1.5fr ' : '';
  return `${checkbox}1.2fr 2fr ${description}auto auto`;
});

// `index` is the row's position in `rows.value` — i.e. its real write-through index
// — carried alongside the row through filtering, never the position in `displayRows` (which a
// filter can move). The trailing blank row is appended last and unconditionally, at
// `index: rows.value.length`: it is the add affordance, not data, and hiding it under a filter
// would make a filtered table un-appendable.
interface DisplayEntry {
  row: T;
  index: number;
}

const displayRows = computed<DisplayEntry[]>(() => {
  const q = props.filterQuery ?? '';
  const filtered = rows.value
    .map((row, index) => ({ row, index }))
    .filter(({ row }) => matchesQuery(row.name, q) || matchesQuery(row.value, q));
  return [...filtered, { row: props.blankRow(), index: rows.value.length }];
});

// Composes with templateToken rather than replacing it — a bare position (outside any
// `{{…}}`) still needs to offer valueCandidatesFor's own vocabulary (`Bearer `, `application/
// json`, …), which templateToken alone would never surface (it returns null there, closing the
// popup). templateToken's own `from` is at least 2 inside a reference (it starts after a `{{`);
// wholeFieldToken's is always 0 — so `from > 0` below is an exact, cheap "are we inside a
// reference" test with no extra state.
function headerValueToken(
  text: string,
  caret: number,
): { from: number; to: number; word: string } | null {
  return templateToken(text, caret) ?? wholeFieldToken(text, caret);
}

function rowValueCandidates(
  row: T,
): (ctx: { text: string; from: number; word: string }) => readonly Completion[] {
  return (ctx) => {
    const variableCandidates = props.valueVariableSupport?.candidates(ctx) ?? [];
    if (ctx.from > 0) return variableCandidates; // inside {{…}} — the reference source owns this
    return [...(props.valueCandidatesFor?.(row.name) ?? []), ...variableCandidates];
  };
}

function updateField(index: number, field: 'name' | 'value' | 'description', value: string): void {
  const next = [...rows.value];
  if (index === next.length) next.push(props.blankRow());
  next[index] = { ...next[index], [field]: value };
  rows.value = next;
}

function toggleEnabled(index: number): void {
  if (index >= rows.value.length) return;
  const next = [...rows.value];
  next[index] = { ...next[index], enabled: !next[index].enabled };
  rows.value = next;
}

function removeRow(index: number): void {
  rows.value = rows.value.filter((_, i) => i !== index);
}

// The trailing blank row IS the add affordance (displayRows' own comment above), so the
// row a user needs next is the one that appears BELOW the one they just filled in — off the fold
// on any table long enough to scroll. `block: 'nearest'` never moves the viewport when the
// row is already visible, which is the common case and must stay a no-op. Guarded on growth only —
// a removal shrinking `rows.length` must leave the viewport alone.
const containerRef = ref<HTMLElement | null>(null);
watch(
  () => rows.value.length,
  (next, prev) => {
    if (next <= prev) return;
    void nextTick().then(() => {
      const last = containerRef.value?.querySelector('.field-row:last-child');
      if (last) revealElement(last);
    });
  },
);

// Arrow-key navigation across this table's rows, one container-level handler —
// not a shared `theme/rowKeyNav.ts`-shaped module: `views/grpcrequest/**` may not import
// `views/httprequest/**` (biome.json), MetadataTable.vue is `views/grpcrequest`'s own literal copy
// of this table (the trade, accepted deliberately), and this handler is small enough that
// duplicating it costs less than the coupling a shared module would create.
//
// Column identity is positional among a row's *text* inputs — `input:not([type=checkbox])`
// excludes the enabled-checkbox column (a real checkbox input, Checkbox.vue) from the
// count, so form-data's own extra content-type field in the trailing slot does not shift the
// mapping for rows that lack it (it has no target there, and navigation into it does
// nothing rather than landing on the wrong field). `, textarea` covers a `grow` cell's own
// `<textarea>` — no cell value can contain a newline, so a cell is still one logical
// line, and this table's own row-stepping contract is unaffected.
function textInputsIn(row: Element): (HTMLInputElement | HTMLTextAreaElement)[] {
  return Array.from(
    row.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
      'input:not([type=checkbox]), textarea',
    ),
  );
}

function onContainerKeydown(e: KeyboardEvent): void {
  // AutocompleteField's own popup binds ArrowUp/ArrowDown while open with preventDefault and no
  // stopPropagation (F4) — this guard is what keeps the popup's own selection move from also
  // moving focus to another row.
  if (e.defaultPrevented) return;
  if (
    e.key !== 'ArrowDown' &&
    e.key !== 'ArrowUp' &&
    e.key !== 'ArrowLeft' &&
    e.key !== 'ArrowRight'
  ) {
    return;
  }
  const el = e.target;
  // Only real text inputs participate — a native select (form-data's kind picker) or a <button>
  // (Choose file, Remove) stays Tab-reachable and keeps its own native arrow behaviour untouched.
  // `.type` exists on both element types (a textarea's own is always the constant 'textarea'),
  // so this one check still excludes only the enabled-checkbox column.
  if (
    !(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) ||
    el.type === 'checkbox'
  )
    return;

  const row = el.closest<HTMLElement>('.field-row');
  const container = row?.parentElement;
  if (!row || !container) return;
  const rows = Array.from(container.children).filter(
    (child): child is HTMLElement =>
      child instanceof HTMLElement && child.classList.contains('field-row'),
  );
  const rowIndex = rows.indexOf(row);
  const inputsInRow = textInputsIn(row);
  const colIndex = inputsInRow.indexOf(el);
  if (colIndex === -1) return;

  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    // No wrap at either end — the trailing blank row is itself a normal target, so ArrowDown from
    // the last real row reaches it exactly as a click would.
    const targetRow = rows[rowIndex + (e.key === 'ArrowDown' ? 1 : -1)];
    const target = targetRow ? textInputsIn(targetRow)[colIndex] : undefined;
    if (!target) return;
    e.preventDefault();
    const offset = Math.min(el.selectionStart ?? 0, target.value.length);
    target.focus();
    target.setSelectionRange(offset, offset);
    return;
  }

  // ArrowLeft/ArrowRight: native everywhere except a field edge — a real (non-collapsed) selection
  // is left to the browser's own left/right-collapses-selection behaviour.
  const start = el.selectionStart;
  const end = el.selectionEnd;
  if (start === null || end === null || start !== end) return;
  if (e.key === 'ArrowRight' && start === el.value.length) {
    const target = inputsInRow[colIndex + 1];
    if (!target) return;
    e.preventDefault();
    target.focus();
    target.setSelectionRange(0, 0);
  } else if (e.key === 'ArrowLeft' && start === 0) {
    const target = inputsInRow[colIndex - 1];
    if (!target) return;
    e.preventDefault();
    target.focus();
    target.setSelectionRange(target.value.length, target.value.length);
  }
}

// P105 §5.1: the table container is not interactive -- binds via VueUse instead of a raw @keydown.
useEventListener(containerRef, 'keydown', onContainerKeydown);
</script>

<template>
  <!-- P16 D13: flex:1 rather than height:100% -- this is no longer always its flex-column
       parent's only child (HttpRequestView.vue's own filter row, when open, is a sibling above
       it), and a percentage height would ignore that sibling's own space and overflow past it. -->
  <div
    ref="containerRef"
    class="flex flex-1 min-h-0 flex-col gap-1 overflow-auto p-1.5"
    :data-testid="containerTestid"
  >
    <!-- P22b D9: a grid, not independent flex items -- named columns (gridTemplateColumns, above)
         keep every row's name/value/trailing/remove cells at the same width regardless of what an
         individual row happens to render inside one of them (F13). -->
    <div
      v-for="entry in displayRows"
      :key="entry.index"
      class="field-row grid items-center gap-1"
      :style="{ gridTemplateColumns }"
      :data-testid="`${testidPrefix}-row`"
    >
      <Checkbox
        v-if="showEnabled"
        :model-value="!!entry.row.enabled"
        :disabled="entry.index >= rows.length"
        :data-testid="`${testidPrefix}-enabled`"
        @update:model-value="toggleEnabled(entry.index)"
      >
        <CodiconIcon name="check" :size="12" />
      </Checkbox>
      <div class="min-w-0">
        <!-- A name is as legal a place for a {{reference}} as a value is
             (`X-{{tenant}}-Id`), and until now it was the one editable Api surface that painted
             none and hovered none. The gate widens from `nameCandidates` alone to "either a name
             vocabulary or variable support", so a params/form-data name — which has no vocabulary
             — still gets the highlight and the resolved-value hover. `token-at` stays
             `wholeFieldToken`: completion here still means "the whole field is one header name",
             which is independent of what `range-highlights`/`hover-at` paint. -->
        <AutocompleteField
          v-if="nameCandidates || valueVariableSupport"
          grow
          class="w-full"
          :model-value="entry.row.name"
          :placeholder="namePlaceholder"
          :data-testid="`${testidPrefix}-name`"
          :candidates="nameCandidates ?? EMPTY_CANDIDATES"
          :token-at="wholeFieldToken"
          :range-highlights="valueVariableSupport?.rangeHighlights"
          :hover-at="valueVariableSupport?.hoverAt"
          @update:model-value="updateField(entry.index, 'name', $event)"
        />
        <InputGroup v-else>
          <InputGroupTextarea
            :model-value="entry.row.name"
            :placeholder="namePlaceholder"
            :data-testid="`${testidPrefix}-name`"
            rows="1"
            @update:model-value="updateField(entry.index, 'name', String($event))"
          />
        </InputGroup>
      </div>
      <slot
        name="value"
        :row="entry.row"
        :index="entry.index"
        :is-trailing="entry.index >= rows.length"
        :update="(v: string) => updateField(entry.index, 'value', v)"
      >
        <div class="min-w-0">
          <AutocompleteField
            v-if="valueVariableSupport"
            grow
            class="w-full"
            :model-value="entry.row.value"
            :placeholder="valuePlaceholder"
            :data-testid="`${testidPrefix}-value`"
            :candidates="
              valueCandidatesFor ? rowValueCandidates(entry.row) : valueVariableSupport.candidates
            "
            :token-at="valueCandidatesFor ? headerValueToken : templateToken"
            :range-highlights="valueVariableSupport.rangeHighlights"
            :hover-at="valueVariableSupport.hoverAt"
            @update:model-value="updateField(entry.index, 'value', $event)"
          />
          <InputGroup v-else>
            <InputGroupTextarea
              :model-value="entry.row.value"
              :placeholder="valuePlaceholder"
              :data-testid="`${testidPrefix}-value`"
              rows="1"
              @update:model-value="updateField(entry.index, 'value', String($event))"
            />
          </InputGroup>
        </div>
      </slot>
      <!-- P22b D6: plain prose about the field, not a value — no autocomplete, no {{variable}}
           colouring. Behind showDescriptions (D7) so a table with no descriptions in it does not
           pay a fourth AutocompleteField-width column for nothing. -->
      <div v-if="showDescriptions" class="min-w-0">
        <InputGroup>
          <InputGroupTextarea
            :model-value="entry.row.description ?? ''"
            :placeholder="t('common.field.description.label')"
            :data-testid="`${testidPrefix}-description`"
            rows="1"
            @update:model-value="updateField(entry.index, 'description', String($event))"
          />
        </InputGroup>
      </div>
      <!-- Wrapped in its own single grid cell — FormDataTable's own trailing content
           (kind select, plus a conditional Choose-file button/caption/remove) varies row to row,
           unlike showEnabled above, so it needs one stable cell to vary *inside* rather than
           shifting the grid's own column count. -->
      <div class="flex min-w-0 items-center gap-1">
        <slot
          name="trailing"
          :row="entry.row"
          :index="entry.index"
          :is-trailing="entry.index >= rows.length"
        />
      </div>
      <TooltipIconButton
        :icon="ICON.close"
        :label="t('common.action.remove.label')"
        :data-testid="`${testidPrefix}-remove`"
        :disabled-trigger="entry.index >= rows.length"
        :disabled="entry.index >= rows.length"
        @click="removeRow(entry.index)"
      />
    </div>
  </div>
</template>
