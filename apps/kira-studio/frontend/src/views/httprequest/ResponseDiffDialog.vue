<script setup lang="ts">
import { statusClass, statusHint } from '@shared/domain/http';
import type { ResponseHistorySnapshot } from '@shared/domain/responseHistory';
import Disclosure from '@theme/components/Disclosure.vue';
import InlineState from '@theme/components/InlineState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import {
  displayValue,
  formatBytes,
  formatDuration,
  formatRelative,
} from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import { KIRA_EDITOR_THEME, loadMonaco } from '@workbench/editor/monaco';
import { monacoLanguageIdFor } from '@workbench/editor/monacoLanguages';
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { type BeautifyResult, beautifyJson, beautifyXml } from '../../beautify';
import { control } from '../../bridge/control';

// P8 D12: two entries, three levels of difference, one dialog. `ids` are the two selections from
// the History list's own checkboxes — this dialog itself decides which is A (older) and which is
// B (newer) by sentAt, never by click order (D12), so the diff's direction is never a surprise.
const props = defineProps<{ ids: [string, string] }>();
const emit = defineEmits<{ close: [] }>();

const snapA = ref<ResponseHistorySnapshot | null>(null);
const snapB = ref<ResponseHistorySnapshot | null>(null);
const loadingSnapshots = ref(true);
const loadError = ref<string | null>(null);
const monacoLoad = useAsyncAction(loadMonaco);
const mergeLoading = monacoLoad.pending;
const mergeHostRef = ref<HTMLElement | null>(null);
type DiffEditor = import('monaco-editor').editor.IStandaloneDiffEditor;
type TextModel = import('monaco-editor').editor.ITextModel;
let diffEditor: DiffEditor | null = null;
let originalModel: TextModel | null = null;
let modifiedModel: TextModel | null = null;

function disposeMergeView(): void {
  diffEditor?.dispose();
  diffEditor = null;
  originalModel?.dispose();
  originalModel = null;
  modifiedModel?.dispose();
  modifiedModel = null;
}

async function loadSnapshots(): Promise<void> {
  try {
    const [s1, s2] = await Promise.all([
      control.historyGet(props.ids[0]),
      control.historyGet(props.ids[1]),
    ]);
    // ISO timestamps compare lexicographically in chronological order (model.FormatISO's fixed
    // UTC width) — no Date parsing needed to pick the older one.
    if (s1.entry.sentAt <= s2.entry.sentAt) {
      snapA.value = s1;
      snapB.value = s2;
    } else {
      snapA.value = s2;
      snapB.value = s1;
    }
  } catch (err) {
    loadError.value = errorText(err);
  } finally {
    loadingSnapshots.value = false;
  }
}
void loadSnapshots();

// D12: "not compared" — a binary body on either side. The dialog still shows the summary and
// headers levels; only the body diff is withheld, with the reason stated inline.
const bothStored = computed(() => !!snapA.value?.bodyStored && !!snapB.value?.bodyStored);

interface DetectedBody {
  format: 'json' | 'xml' | null;
  result: BeautifyResult | null;
}

// beautifyJson/beautifyXml run the exact same parse scanJson/scanXml
// would — detected once per body here (memoized by `computed` below) and reused by both
// commonFormat's own detection and bodyTextA/B's rendering, instead of parsing each body twice.
// The `<…>` bracket check mirrors ResponsePane.vue's own (and celleditor/detect.ts's own
// detectXml gate): an XML parse alone accepts plain text with no tags at all, so without it every
// plain-text response would misreport as XML.
function detectAndBeautify(body: string): DetectedBody {
  const json = beautifyJson(body, 'indented');
  if (json.ok) return { format: 'json', result: json };
  const t = body.trim();
  if (t.length > 0 && t[0] === '<' && t[t.length - 1] === '>') {
    const xml = beautifyXml(body, 'indented');
    if (xml.ok) return { format: 'xml', result: xml };
  }
  return { format: null, result: null };
}

const detectedA = computed<DetectedBody | null>(() =>
  snapA.value ? detectAndBeautify(snapA.value.response.body) : null,
);
const detectedB = computed<DetectedBody | null>(() =>
  snapB.value ? detectAndBeautify(snapB.value.response.body) : null,
);

// Both bodies are pretty-printed before diffing only when *both* are the same recognised
// format — a minified-vs-minified diff of two 40 KB single lines tells the user nothing.
// beautifyJson(text, 'indented') is lossless by construction, so this never
// misrepresents what came back.
const commonFormat = computed<'json' | 'xml' | null>(() => {
  const a = detectedA.value?.format ?? null;
  const b = detectedB.value?.format ?? null;
  return a && a === b ? a : null;
});

const bodyTextA = computed(() => {
  if (!snapA.value) return '';
  return commonFormat.value && detectedA.value?.result
    ? detectedA.value.result.text
    : snapA.value.response.body;
});
const bodyTextB = computed(() => {
  if (!snapB.value) return '';
  return commonFormat.value && detectedB.value?.result
    ? detectedB.value.result.text
    : snapB.value.response.body;
});

// Level 2: the headers table. Reduced to added/removed/changed/unchanged by header *name*
// (case-insensitive) rather than by the diff editor's own text-diff — headers are a keyed
// structure, not ordered prose, so a name-keyed comparison is the semantically correct model (it
// stays right even when two servers emit the same headers in a different order) and needs no
// diff algorithm of its own. Monaco's diff editor is exactly what the body level below is for.
interface HeaderRow {
  name: string;
  a: string | null;
  b: string | null;
  status: 'added' | 'removed' | 'changed' | 'unchanged';
}
const HEADER_STATUS_KEYS: Record<HeaderRow['status'], MessageKey> = {
  added: 'common.added.status',
  removed: 'common.removed.status',
  changed: 'common.changed.status',
  unchanged: 'studio.http.diff.unchangedRow.status',
};
function headerStatusText(status: HeaderRow['status']): string {
  return t(HEADER_STATUS_KEYS[status]);
}
const HEADER_STATUS_CLASS: Record<HeaderRow['status'], string> = {
  added: 'text-ok',
  removed: 'text-error',
  changed: 'text-warn',
  unchanged: '',
};
const headerRows = computed<HeaderRow[]>(() => {
  if (!snapA.value || !snapB.value) return [];
  const order: string[] = [];
  const mapA = new Map<string, string>();
  const mapB = new Map<string, string>();
  for (const h of snapA.value.response.headers) {
    const key = h.name.toLowerCase();
    if (!mapA.has(key)) {
      mapA.set(key, h.value);
      order.push(h.name);
    }
  }
  for (const h of snapB.value.response.headers) {
    const key = h.name.toLowerCase();
    if (!mapA.has(key) && !mapB.has(key)) order.push(h.name);
    if (!mapB.has(key)) mapB.set(key, h.value);
  }
  return order.map((name) => {
    const key = name.toLowerCase();
    const a = mapA.has(key) ? (mapA.get(key) ?? null) : null;
    const b = mapB.has(key) ? (mapB.get(key) ?? null) : null;
    let status: HeaderRow['status'];
    if (a === null) status = 'added';
    else if (b === null) status = 'removed';
    else if (a === b) status = 'unchanged';
    else status = 'changed';
    return { name, a, b, status };
  });
});
const changedHeaderRows = computed(() => headerRows.value.filter((r) => r.status !== 'unchanged'));
const unchangedHeaderRows = computed(() =>
  headerRows.value.filter((r) => r.status === 'unchanged'),
);

// The real payoff of Monaco's diff editor — a scroll-locked,
// line-aligned, intra-line-highlighted side-by-side view. Both sides read-only (the same
// second-layer guard RepoDiffView.vue's review diff already uses: readOnly alone blocks edits,
// renderMarginRevertIcon/renderGutterMenu additionally hide the revert/apply affordances a
// read-write diff widget would otherwise offer).
async function buildMergeView(): Promise<void> {
  if (!bothStored.value || !snapA.value || !snapB.value) return;
  if (!(await monacoLoad.run())) return;
  const mod = await loadMonaco();
  disposeMergeView();
  await nextTick();
  if (!mergeHostRef.value) return;
  const languageId = monacoLanguageIdFor(commonFormat.value ?? 'plain');
  originalModel = mod.editor.createModel(bodyTextA.value, languageId);
  modifiedModel = mod.editor.createModel(bodyTextB.value, languageId);
  diffEditor = mod.editor.createDiffEditor(mergeHostRef.value, {
    theme: KIRA_EDITOR_THEME,
    readOnly: true,
    domReadOnly: true,
    originalEditable: false,
    renderMarginRevertIcon: false,
    renderGutterMenu: false,
    automaticLayout: true,
    renderSideBySide: true,
    renderIndicators: true,
    hideUnchangedRegions: { enabled: true },
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
  });
  diffEditor.setModel({ original: originalModel, modified: modifiedModel });
  // tests/ui's own text-reading seam — both models are static once created (this
  // dialog never edits them), so a one-time attribute set here (rather than a change listener,
  // MonacoHost.vue's own approach) is enough. The diff editor's two sides are plain Monaco
  // sub-editors, not a MonacoHost mount, so they carry no `data-kira-editor-text` of their own.
  if (__KIRA_DEBUG_HOOKS__ && mergeHostRef.value) {
    mergeHostRef.value.setAttribute('data-kira-diff-original-text', originalModel.getValue());
    mergeHostRef.value.setAttribute('data-kira-diff-modified-text', modifiedModel.getValue());
  }
}

watch(
  () => loadingSnapshots.value,
  (loading) => {
    if (!loading && bothStored.value) void buildMergeView();
  },
);

onUnmounted(() => {
  disposeMergeView();
});
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent size="2xl" fixed-height data-testid="http-diff-dialog">
      <DialogHeader close-testid="http-diff-close">
        <DialogTitle>{{ t('studio.http.diff.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody flush>
        <InlineState v-if="loadingSnapshots" kind="loading">{{
          t('common.state.loading.loading')
        }}</InlineState>
        <Alert v-else-if="loadError" variant="error"
          ><AlertDescription>{{ loadError }}</AlertDescription></Alert
        >
        <div v-else-if="snapA && snapB" class="flex h-full min-h-0 flex-col">
          <div class="flex shrink-0 items-center gap-2 px-2 py-1.5" data-testid="http-diff-summary">
            <div class="flex flex-col gap-0.5">
              <Tooltip>
                <TooltipTrigger as-child>
                  <span :class="[rowMeta, 'self-start']">{{
                    formatRelative(snapA.entry.sentAt)
                  }}</span>
                </TooltipTrigger>
                <TooltipContent>{{ snapA.entry.sentAt }}</TooltipContent>
              </Tooltip>
              <div class="flex items-center gap-1">
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Badge
                      data
                      :variant="statusClass(snapA.entry.status)"
                      data-testid="http-diff-status-a"
                    >
                      {{ snapA.entry.status }} {{ snapA.entry.statusText }}
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent>{{ statusHint(snapA.entry.status) }}</TooltipContent>
                </Tooltip>
                <span :class="rowMeta">{{ formatDuration(snapA.entry.elapsedMs) }}</span>
                <span :class="rowMeta">{{ formatBytes(snapA.entry.bodyBytes) }}</span>
              </div>
            </div>
            <span class="text-muted-foreground">→</span>
            <div class="flex flex-col gap-0.5">
              <Tooltip>
                <TooltipTrigger as-child>
                  <span :class="[rowMeta, 'self-start']">{{
                    formatRelative(snapB.entry.sentAt)
                  }}</span>
                </TooltipTrigger>
                <TooltipContent>{{ snapB.entry.sentAt }}</TooltipContent>
              </Tooltip>
              <div class="flex items-center gap-1">
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Badge
                      data
                      :variant="statusClass(snapB.entry.status)"
                      data-testid="http-diff-status-b"
                    >
                      {{ snapB.entry.status }} {{ snapB.entry.statusText }}
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent>{{ statusHint(snapB.entry.status) }}</TooltipContent>
                </Tooltip>
                <span :class="rowMeta">{{ formatDuration(snapB.entry.elapsedMs) }}</span>
                <span :class="rowMeta">{{ formatBytes(snapB.entry.bodyBytes) }}</span>
              </div>
            </div>
          </div>

          <div class="max-h-40 shrink-0 overflow-auto px-2 py-1" data-testid="http-diff-headers">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead class="w-18"></TableHead>
                  <TableHead class="w-40"></TableHead>
                  <TableHead>{{ t('studio.http.diff.before.label') }}</TableHead>
                  <TableHead>{{ t('studio.http.diff.after.label') }}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow
                  v-for="row in changedHeaderRows"
                  :key="row.name"
                  data-testid="http-diff-header-row"
                >
                  <TableCell class="text-kira-sm" :class="HEADER_STATUS_CLASS[row.status]">{{
                    headerStatusText(row.status)
                  }}</TableCell>
                  <TableCell class="font-data text-muted-foreground">{{ row.name }}</TableCell>
                  <TableCell class="font-data wrap-anywhere">{{ displayValue(row.a) }}</TableCell>
                  <TableCell class="font-data wrap-anywhere">{{ displayValue(row.b) }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
            <Disclosure
              v-if="unchangedHeaderRows.length > 0"
              :label="t('studio.http.diff.unchanged.status', { count: unchangedHeaderRows.length })"
              trigger-class="text-kira-sm text-subtle"
            >
              <Table>
                <TableBody>
                  <TableRow
                    v-for="row in unchangedHeaderRows"
                    :key="row.name"
                    data-testid="http-diff-header-row-unchanged"
                  >
                    <TableCell class="w-18 text-kira-sm" :class="HEADER_STATUS_CLASS[row.status]">{{
                      headerStatusText(row.status)
                    }}</TableCell>
                    <TableCell class="w-40 font-data text-muted-foreground">{{
                      row.name
                    }}</TableCell>
                    <TableCell class="font-data wrap-anywhere">{{ row.a }}</TableCell>
                    <TableCell class="font-data wrap-anywhere">{{ row.b }}</TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </Disclosure>
          </div>

          <Alert v-if="!bothStored" variant="info" data-testid="http-diff-not-comparable">
            <AlertDescription>
              {{ t('studio.http.diff.notKept.description') }}
            </AlertDescription>
          </Alert>
          <template v-else>
            <FieldDescription v-if="!commonFormat" class="shrink-0 px-2 py-1">{{
              t('studio.http.diff.rawBytes.description')
            }}</FieldDescription>
            <InlineState v-if="mergeLoading" kind="loading">{{
              t('studio.http.diff.loadingMerge.loading')
            }}</InlineState>
            <div
              ref="mergeHostRef"
              class="diff-merge-host flex-1 min-h-0 overflow-auto"
              data-testid="http-diff-merge"
            ></div>
          </template>
        </div>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="http-diff-close"
          @click="emit('close')"
          >{{
            t('common.action.close.label')
          }}</Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
@reference "@theme/base.css";
/* P110 I2-17: header status colors moved to HEADER_STATUS_CLASS lookup map on the status span.
   `.diff-merge-host` stays a bare marker to anchor this :deep() rule -- can't be expressed as a
   template utility since it targets a child Monaco renders into the ref'd div. */
.diff-merge-host :deep(.monaco-diff-editor) {
  @apply h-full;
}
</style>
