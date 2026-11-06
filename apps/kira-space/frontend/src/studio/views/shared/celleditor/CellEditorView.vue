<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import { pathTail } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { formatBytes } from '@workbench/behaviour/format';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import MonacoDiff from '@workbench/editor/MonacoDiff.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue';
import type { ConsoleDiagnostic } from '../../../editor/diagnostics';
import { findRanges } from '../../../editor/findRanges';
import MonacoHost from '../../../editor/MonacoHost.vue';
import { cellKey, type SelectedCell, useCellSelectionStore } from '../../../state/cellSelection';
import { useConnectionsStore } from '../../../state/connections';
import { typeClassTextClass } from '../../../theme/icons';
import EditBufferActions from '../EditBufferActions.vue';
import ResponseFindBar, { type FindBarHost, type FindBarTarget } from '../ResponseFindBar.vue';
import { sqlDialectFor } from '../sqlIdent';
import { typeDescription } from '../typeGlossary';
import { useEditBuffer } from '../useEditBuffer';
import { decodeToText, encodeFromText } from './binary';
import CellGenerateMenu from './CellGenerateMenu.vue';
import { describeValue, detectFormat, type FormatGuess } from './detect';
import {
  beautifyFor,
  type CellFormat,
  canBeautify,
  FORMAT_GROUPS,
  FORMAT_LANGUAGE,
  formatAutoLabel,
  formatHelp,
  formatLabel,
} from './formats';
import type { Generator } from './generate';
import { readOnlyChipText, readOnlyChipTitle } from './readOnlyChip';
import { useCellEditorFormatStore } from './state';
import TimestampPane from './TimestampPane.vue';
import { useCellDiff } from './useCellDiff';
import { useDockReady } from './useDockReady';
import { validateFormat } from './validate';

const cellEditorFormatStore = useCellEditorFormatStore();
const cellSelectionStore = useCellSelectionStore();
const contextMenuStore = useContextMenuStore();
const connectionsStore = useConnectionsStore();

const statusEncoder = new TextEncoder();

const props = withDefaults(defineProps<{ cell: SelectedCell; readonly?: boolean }>(), {
  readonly: false,
});
const emit = defineEmits<{ ready: [] }>();
const selectedCell = computed(() => props.cell);
const viewerMode = computed(() => props.readonly === true);

const override = computed<CellFormat | null>(() =>
  cellEditorFormatStore.overrideFor(selectedCell.value),
);

const isNullValue = computed(() => selectedCell.value.value === null);
const isEmptyValue = computed(() => selectedCell.value.value === '');
const isTruncatedValue = computed(() => selectedCell.value.truncated);

// Detection reads `detectionText`, not `doc`: translate panes round-trip bytes through `doc`, and
// re-detecting those intermediate encodings could hide the pane being typed into.
const detected = computed<FormatGuess[]>(() => {
  const c = selectedCell.value;
  if (c.value === null) return [];
  return detectFormat({
    text: detectionText.value,
    typeClass: c.column.typeClass,
    dataType: c.column.dataType,
    columnName: c.column.name,
  });
});
const detectedFormat = computed<CellFormat>(() => detected.value[0]?.format ?? 'text');
const detectedReason = computed<string>(() => detected.value[0]?.reason ?? '');

const effectiveFormat = computed<CellFormat>(() => override.value ?? detectedFormat.value);
const language = computed<EditorLanguageId>(() => FORMAT_LANGUAGE[effectiveFormat.value]);

const sqlDialect = computed(() => {
  const record = connectionsStore.connectionRecord(selectedCell.value.connectionId);
  return sqlDialectFor(record?.kind);
});

const readOnlyReason = computed(() =>
  viewerMode.value ? null : cellEditorFormatStore.readOnlyReasonFor(selectedCell.value),
);

const isEditable = computed(
  () => !viewerMode.value && readOnlyReason.value === null && !!selectedCell.value.onEdit,
);
const chipText = computed(() => readOnlyChipText(readOnlyReason.value));
const chipTitle = computed(() => readOnlyChipTitle(readOnlyReason.value));

const buffer = useEditBuffer({
  original: () => selectedCell.value.value ?? '',
  beautifier: () =>
    canBeautify(effectiveFormat.value)
      ? (text, mode) => beautifyFor(effectiveFormat.value, text, mode)
      : null,
  onRevert: () => selectedCell.value.onRevert?.(),
});
const { doc, isDirty, formatted, beautifyFailure, writeDoc } = buffer;
// A staged cell republishes with its staged value as `original()`, so the buffer reads clean;
// Revert must stay enabled to un-stage it.
const actionsBuffer = {
  ...buffer,
  isDirty: computed(() => isDirty.value || selectedCell.value.original !== undefined),
};

// What `detected` above reads — deliberately a separate ref from `doc`, not an alias for
// it, and deliberately not updated on every keystroke either. `doc` also receives byte-level
// round-trip writes from the decoded-text pane (onDecodedInput) and TimestampPane, each re-encoding
// a pane-managed value that is *already* known to be that exact format; re-running detection
// against those intermediate encodings (or even the main editor's own direct edits, once a
// translate-pane format is already in effect) risks the pane's own output falling out of its own
// format mid-keystroke — base64's 20-char floor rejects plenty of genuinely valid short base64 —
// which would hide the very pane the user is typing into. Once the effective format is one of
// PINNED_FORMATS (hex/base64/the three timestamp encodings — every format with a translate pane),
// it stays put for the rest of that cell's editing session;
// only the plain read-it-directly formats (text/json/xml/csv/sql, none of which own a pane) keep
// re-detecting live.
const PINNED_FORMATS = new Set<CellFormat>([
  'hex',
  'base64',
  'iso8601',
  'epochSeconds',
  'epochMillis',
]);
const detectionText = ref(selectedCell.value.value ?? '');

function onMainDocInput(text: string): void {
  doc.value = text;
  if (!PINNED_FORMATS.has(effectiveFormat.value)) detectionText.value = text;
}

let lastKey: string | null = null;
let lastValue: string | null = null;

watch(
  selectedCell,
  (c, prevCell) => {
    const key = cellKey(c);
    if (key === lastKey && c.value === lastValue) return;
    // Judge dirtiness against `prevCell.value`: `original()` already reads the new cell.
    const wasDirty = prevCell ? doc.value !== (prevCell.value ?? '') : false;
    if (
      wasDirty &&
      prevCell &&
      !viewerMode.value &&
      prevCell.onEdit &&
      cellEditorFormatStore.readOnlyReasonFor(prevCell) === null
    ) {
      prevCell.onEdit(doc.value);
    }
    lastKey = key;
    lastValue = c.value;
    buffer.reseed();
    detectionText.value = c.value ?? '';
  },
  { immediate: true },
);

const isTimestampFormat = computed(
  () =>
    effectiveFormat.value === 'epochSeconds' ||
    effectiveFormat.value === 'epochMillis' ||
    effectiveFormat.value === 'iso8601',
);

const showDecodedPane = computed(
  () => effectiveFormat.value === 'hex' || effectiveFormat.value === 'base64',
);
const encodedClass = computed(() => {
  if (showDecodedPane.value) return 'flex-1 basis-7/12 min-h-0';
  if (isTimestampFormat.value) return 'flex-1 basis-0 min-h-0';
  return 'flex-auto min-h-0';
});
const decodedDoc = ref<string | null>('');
let skipNextDecode = false;

function syncDecodedFromDoc(): void {
  if (!showDecodedPane.value) {
    decodedDoc.value = '';
    return;
  }
  decodedDoc.value = decodeToText(effectiveFormat.value as 'hex' | 'base64', doc.value);
}

watch(
  [doc, effectiveFormat],
  () => {
    if (skipNextDecode) {
      skipNextDecode = false;
      return;
    }
    syncDecodedFromDoc();
  },
  { immediate: true },
);

function onDecodedInput(text: string): void {
  decodedDoc.value = text;
  if (!showDecodedPane.value) return;
  const next = encodeFromText(effectiveFormat.value as 'hex' | 'base64', text, doc.value);
  if (writeDoc(next)) skipNextDecode = true;
}

function saveEdit(): void {
  const c = selectedCell.value;
  if (!isEditable.value || !c.onEdit) return;
  c.onEdit(doc.value);
}

// Auto-stages on blur, matching the deleted DataGrid.vue's own inline double-click edit (its
// `commitEdit` fired on the same event) — no separate Save button needed since leaving the editor
// is already
// the "I'm done with this value" signal, and the grid's own pending-edit row highlighting is the
// feedback that it landed. `focusout` (not `blur`, which doesn't bubble) on the wrapping div.
//
// `focusout` fires (and bubbles) whenever the *previously* focused element loses focus, whether
// the next focus target is outside this div or just another control inside it (the timestamp
// field -> the encoded box, the zone toggle, the calendar) — it says nothing on its own about
// whether focus left the panel. `relatedTarget` is the element gaining focus, so only
// treat this as "done editing" when that target is null (focus left the window/app entirely) or
// sits outside the wrapping div; a same-panel transition must fall through and stage nothing.
function onEditorBlur(e: FocusEvent): void {
  const next = e.relatedTarget as Node | null;
  const container = e.currentTarget as HTMLElement | null;
  if (next && container?.contains(next)) return;
  if (isEditable.value && isDirty.value) saveEdit();
}

onBeforeUnmount(() => {
  if (isEditable.value && isDirty.value) saveEdit();
});

function onEditorKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault();
    buffer.reset();
  }
}

const editorBodyEl = useTemplateRef<HTMLElement>('editorBodyEl');
useEventListener(editorBodyEl, 'keydown', onEditorKeydown);
useShortcut(editorBodyEl, ['form.save', 'form.submit'], saveEdit, { capture: true });
useEventListener(editorBodyEl, 'focusout', onEditorBlur);

function closePanel(): void {
  cellSelectionStore.clearSelectedCellFor(selectedCell.value.tabId);
}

const {
  available: diffAvailable,
  shown: diffShown,
  sides: diffSides,
  toggle: toggleDiff,
  close: closeDiff,
} = useCellDiff(() => selectedCell.value, doc, isDirty);

const { markMainReady, markDecodedReady } = useDockReady(diffShown, showDecodedPane, () =>
  emit('ready'),
);

// Find targets the main editor, absent in the diff view, so the two are exclusive.
const findOpen = ref(false);
function toggleFind(): void {
  closeDiff();
  findOpen.value = !findOpen.value;
}
function onToggleDiff(): void {
  findOpen.value = false;
  toggleDiff();
}
function closeFind(): void {
  findOpen.value = false;
}
const encodedHostRef = ref<FindBarHost | null>(null);
const findBarRef = ref<{ query: string; currentGlobal: number } | null>(null);
const findTargets = computed<readonly FindBarTarget[]>(() => {
  if (!findOpen.value) return [];
  return [{ doc: doc.value, host: encodedHostRef.value }];
});
const docHighlights = computed(() => {
  const bar = findBarRef.value;
  const query = bar?.query ?? '';
  if (!query || findTargets.value.length === 0) return undefined;
  const currentGlobal = bar?.currentGlobal ?? -1;
  return (text: string) => findRanges(text, query, currentGlobal);
});

const formatProblem = computed(() =>
  isNullValue.value
    ? null
    : validateFormat(effectiveFormat.value, doc.value, isTruncatedValue.value),
);

function cellLintSource(): ConsoleDiagnostic[] {
  const problem = formatProblem.value;
  if (!problem) return [];
  const text = doc.value;
  const from = Math.min(problem.offset ?? 0, text.length);
  const to = problem.offset === undefined ? text.length : Math.min(from + 1, text.length);
  return [{ from, to: Math.max(to, from), severity: 'error', message: problem.message }];
}

const formatHint = computed(() => formatHelp(effectiveFormat.value));

function setFormat(format: CellFormat | null): void {
  cellEditorFormatStore.setOverride(selectedCell.value, format);
}

function openFormatMenu(e: MouseEvent): void {
  if (isNullValue.value) return;
  const rows: MenuItem[] = [
    {
      type: 'item',
      id: 'format-auto',
      label: formatAutoLabel(detectedFormat.value),
      checked: override.value === null,
      hint: detectedReason.value || undefined,
      run: () => setFormat(null),
    },
  ];
  FORMAT_GROUPS.forEach((group, i) => {
    if (i > 0) rows.push({ type: 'separator' });
    for (const f of group) {
      rows.push({
        type: 'item',
        id: `format-${f}`,
        label: formatLabel(f),
        checked: override.value === f,
        hint: formatHelp(f),
        run: () => setFormat(f),
      });
    }
  });
  contextMenuStore.openContextMenu(e, rows);
}

function applyGenerator(gen: Generator): void {
  if (!isEditable.value) return;
  doc.value = gen.run(effectiveFormat.value);
  saveEdit();
}

const targetLabel = computed(() => {
  const c = selectedCell.value;
  const tail = pathTail(c.path);
  return `${tail?.name ?? c.path}.${c.column.name}`;
});

// Same glossary the grid's own column header reads (columnHeaderTooltip, SlickGridHost.vue) — hovering
// the data-type badge here should explain the type exactly as hovering the header already does.
const dataTypeHint = computed(
  () => typeDescription(selectedCell.value.column.dataType) ?? undefined,
);

const dataTypeClass = computed(() => typeClassTextClass(selectedCell.value.column.typeClass));

const statusLine = computed(() => {
  if (isNullValue.value) return t('studio.common.nullValue.label');
  const value = doc.value;
  const parts: string[] = [];
  parts.push(formatBytes(statusEncoder.encode(value).length));
  const reading = describeValue(effectiveFormat.value, value);
  if (reading) parts.push(reading);
  if (isTruncatedValue.value)
    parts.push(t('studio.cellEditor.truncatedPrefix.status', { size: formatBytes(64 * 1024) }));
  if (beautifyFailure.value) parts.push(beautifyFailure.value);
  return parts.join(' · ');
});
</script>

<template>
  <div
    class="flex flex-col h-full min-h-0"
    data-testid="cell-editor-panel"
    :data-cell-key="cellKey(selectedCell)"
    :data-format="effectiveFormat"
    :data-detected="detectedFormat"
    :data-read-only="viewerMode || undefined"
    :data-read-only-reason="readOnlyReason"
    :data-formatted="formatted"
    :data-dirty="isDirty"
    :data-invalid="!!formatProblem || undefined"
  >
    <ViewToolbar>
      <span class="size-4 flex items-center justify-center shrink-0">
        <CodiconIcon name="symbol-string" :size="13" />
      </span>
      <span class="text-kira-md text-fg truncate" data-testid="cell-editor-target">{{
        t('studio.cellEditor.target.label', { target: targetLabel, row: selectedCell.row + 1 })
      }}</span>
      <Tooltip v-if="dataTypeHint">
        <TooltipTrigger as-child>
          <Badge data :class="dataTypeClass">{{ selectedCell.column.dataType }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{ dataTypeHint }}</TooltipContent>
      </Tooltip>
      <Badge data v-else :class="dataTypeClass">{{ selectedCell.column.dataType }}</Badge>
      <Badge v-if="isNullValue" variant="info" data-testid="cell-editor-badge-null">{{
        t('studio.common.nullValue.label')
      }}</Badge>
      <Badge v-if="isEmptyValue" variant="info" data-testid="cell-editor-badge-empty">{{
        t('studio.cellEditor.empty.status')
      }}</Badge>
      <Badge v-if="isTruncatedValue" variant="warn" data-testid="cell-editor-badge-truncated">{{
        t('studio.common.truncated.status')
      }}</Badge>
      <Tooltip>
        <TooltipTrigger as-child>
          <Badge class="max-w-56 truncate" data-testid="cell-editor-status">{{ statusLine }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{ statusLine }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="formatProblem">
        <TooltipTrigger as-child>
          <Badge variant="error" class="max-w-56 truncate" data-testid="cell-editor-invalid">{{
            formatProblem.message
          }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{ formatProblem.message }}</TooltipContent>
      </Tooltip>

      <!-- the format select + beautify/reset trio: this panel's own controls, set off from the
           identity badges with the standard s-4 gutter (mirrors CellEditor.html's inline group) -->
      <span class="flex items-center shrink-0 gap-1.5 ml-2">
        <Tooltip>
          <TooltipTrigger as-child>
            <TooltipDisabledTrigger>
              <PickerTrigger
                size="kira"
                class="max-w-40 font-ui"
                data-testid="cell-editor-format"
                :disabled="isNullValue"
                :label="override ? formatLabel(override) : formatAutoLabel(detectedFormat)"
                @click="openFormatMenu"
              />
            </TooltipDisabledTrigger>
          </TooltipTrigger>
          <TooltipContent>{{ formatHint }}</TooltipContent>
        </Tooltip>

        <template v-if="!viewerMode">
          <CellGenerateMenu :disabled="!isEditable" @apply="applyGenerator" />
          <EditBufferActions
            :buffer="actionsBuffer"
            testid-prefix="cell-editor"
            :show-bytes="false"
          />
          <TooltipIconButton
            icon="diff"
            :label="t('git.menu.stashShow.label')"
            :pressed="diffShown"
            disabled-trigger
            :disabled="!diffAvailable"
            data-testid="cell-editor-diff-toggle"
            @click="onToggleDiff"
          />
        </template>
      </span>

      <span class="ml-auto flex items-center gap-1">
        <Tooltip v-if="readOnlyReason && chipTitle">
          <TooltipTrigger as-child>
            <Badge variant="warn">
              <CodiconIcon name="lock" :size="13" />
              {{ chipText }}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{{ chipTitle }}</TooltipContent>
        </Tooltip>
        <Badge v-else-if="readOnlyReason" variant="warn">
          <CodiconIcon name="lock" :size="13" />
          {{ chipText }}
        </Badge>
        <TooltipIconButton
          icon="search"
          :label="t('studio.cellEditor.find.tooltip')"
          :pressed="findOpen"
          data-testid="cell-editor-search-toggle"
          @click="toggleFind"
        />
        <TooltipIconButton
          icon="close"
          :label="t('common.action.close.label')"
          data-testid="cell-editor-close"
          @click="closePanel"
        />
      </span>
    </ViewToolbar>

    <div ref="editorBodyEl" class="flex-1 min-h-0 flex flex-col overflow-hidden">
      <div v-if="diffShown" class="flex-1 min-h-0" data-testid="cell-editor-diff-pane">
        <MonacoDiff data-testid="cell-editor-diff" v-bind="diffSides" />
      </div>
      <template v-else>
        <div :class="encodedClass" data-testid="cell-editor-encoded">
          <MonacoHost
            ref="encodedHostRef"
            :doc="doc"
            :language="language"
            :sql-dialect="sqlDialect"
            :readonly="!isEditable"
            :lint-source="cellLintSource"
            :range-highlights="docHighlights"
            @update:doc="onMainDocInput"
            @ready="markMainReady"
          />
        </div>

        <!-- Hex/base64: the same bytes as editable plaintext, kept in lockstep with the encoded box
             above in both directions (encode<->decode, see onDecodedInput). -->
        <template v-if="showDecodedPane">
          <div
            class="flex shrink-0 items-center gap-1 bg-elevated text-subtle text-kira-sm leading-4 py-0.5 px-2"
          >
            <CodiconIcon name="symbol-string" :size="13" />
            <span>{{ t('studio.cellEditor.decoded.label') }}</span>
          </div>
          <div
            v-if="decodedDoc !== null"
            class="flex-1 basis-5/12 min-h-0"
            data-testid="cell-editor-decoded"
          >
            <MonacoHost
              :doc="decodedDoc"
              language="plain"
              :readonly="!isEditable"
              @update:doc="onDecodedInput"
              @ready="markDecodedReady"
            />
          </div>
          <Alert
            v-else
            variant="info"
            class="flex-1 basis-5/12 min-h-0 overflow-auto items-center text-subtle"
            data-testid="cell-editor-decoded-empty"
          >
            <AlertDescription>
              {{
                t('studio.cellEditor.decoded.invalid.hint', {
                  format: formatLabel(effectiveFormat),
                })
              }}
            </AlertDescription>
          </Alert>
        </template>

        <!-- The three timestamp formats: TimestampPane owns its own readings, zone switch, editable
             field and calendar entirely — this file only decides whether to show it. -->
        <template v-else-if="isTimestampFormat">
          <div
            class="flex shrink-0 items-center gap-1 bg-elevated text-subtle text-kira-sm leading-4 py-0.5 px-2"
          >
            <CodiconIcon name="calendar" :size="13" />
            <span>{{ t('studio.cellEditor.timestamp.title') }}</span>
          </div>
          <TimestampPane
            class="shrink-0"
            :doc="doc"
            :format="effectiveFormat"
            :readonly="!isEditable"
            @update:doc="writeDoc($event)"
          />
        </template>
      </template>
    </div>

    <ResponseFindBar v-if="findOpen" ref="findBarRef" :targets="findTargets" @close="closeFind" />
  </div>
</template>
