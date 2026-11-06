<script setup lang="ts">
import { defaultContentTypeFor, generateRawRequestFromStored } from '@kira/api-core';
import type { HttpCodeLanguage, HttpResponseWire, HttpWireFidelity } from '@shared/domain/http';
import EmptyState from '@theme/components/EmptyState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { type MessageKey, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { copyText } from '@workbench/behaviour/clipboard';
import { formatBytes } from '@workbench/behaviour/format';
import type { RangeHighlight } from '@workbench/editor/ranges';
import { computed, ref } from 'vue';
import type { HttpRequestTabRecord } from '../../../state/tabDomain';
import MonacoHost from '../../editor/MonacoHost.vue';
import type { FindBarHost } from '../shared/ResponseFindBar.vue';
import { useHttpHistoryViewing } from './history';
import { useHttpRequestViewStore } from './state';

// P9 D12/D14/D15: the inspector — the SPEC's own "view the exact bytes sent and received", with
// its fidelity stated rather than assumed. F16: lives here (views/httprequest/), not http/, because
// it is mounted from inside ResponsePane.vue and needs MonacoHost/theme/primitives.
const props = defineProps<{
  tab: HttpRequestTabRecord;
  /** ResponseFindBar's own painted matches for each of this pane's two documents — the
   *  bar itself has no template access to these editors, since they're this component's own
   *  children. Absent (no find bar open) paints nothing, same as every other rangeHighlights
   *  caller when its prop is absent. */
  requestHighlights?: (doc: string) => readonly RangeHighlight[];
  responseHighlights?: (doc: string) => readonly RangeHighlight[];
}>();

// P8 D10's own source swap, duplicated here rather than threaded down as props — the same
// "each pane computes its own runtime over the tab id" shape ResponseHistoryList.vue already
// uses, so a future pane needs no prop-plumbing change to this component's siblings.
const httpRequestViewStore = useHttpRequestViewStore();
const rt = computed(() => httpRequestViewStore.runtime[props.tab.id]);
const { viewing: viewingStored } = useHttpHistoryViewing(() => props.tab.id);
const response = computed(
  () => viewingStored.value?.snapshot.response ?? rt.value?.response ?? null,
);
const wire = computed(() => response.value?.wire ?? null);

// A stored entry never has a `wire` (it is nulled before persisting), which is why the Raw segment
// used to render nothing at all while viewing one — the stored request/response are sitting right
// there in the snapshot with nothing rendering either. This reconstructs both documents from the
// four fields the snapshot carries, generated in the renderer rather than stored (the objection —
// storing rendered text would double a snapshot — does not apply here: nothing new is stored, only
// computed from what already is).
const showReconstructed = computed(() => !wire.value && !!viewingStored.value);

const storedRequestText = computed(() => {
  const req = viewingStored.value?.snapshot.request;
  if (!req) return '';
  // req.body.codeLanguage is the wire's plain `string` (HttpBodyWire), not the narrow
  // HttpCodeLanguage union tab state carries — this is this app's own stored data, always one of
  // the four, so the cast is safe (defaultContentTypeFor's Record falls back to undefined -> ''
  // for anything else, the same as an unset content type).
  return generateRawRequestFromStored(
    req,
    defaultContentTypeFor(req.body.mode, req.body.codeLanguage as HttpCodeLanguage),
  );
});

function buildResponseHead(r: HttpResponseWire): string {
  const lines = [`${r.proto} ${r.status} ${r.statusText}`];
  for (const h of r.headers) lines.push(`${h.name}: ${h.value}`);
  return lines.join('\n');
}

const storedResponseText = computed(() => {
  if (!showReconstructed.value || !response.value) return '';
  return `${buildResponseHead(response.value)}\n\n${response.value.body}`;
});

// P18 D8: requestBodyStorageTruncated finally gets a reader (F8) — this is the one place a
// stored entry's request can be seen at all, so it is also the one place this flag can be shown.
const requestBodyStorageTruncated = computed(
  () => viewingStored.value?.snapshot.requestBodyStorageTruncated ?? false,
);
const requestFieldsElided = computed(
  () => viewingStored.value?.snapshot.requestFieldsElided ?? false,
);

const FIDELITY_KEYS: Readonly<Record<HttpWireFidelity, MessageKey>> = {
  exact: 'studio.http.raw.exact.description',
  http2: 'studio.http.raw.http2.description',
  proxied: 'studio.http.raw.proxied.description',
};
const fidelityText = computed(() => (wire.value ? t(FIDELITY_KEYS[wire.value.fidelity]) : ''));
// D3 calls for an "info" tone on 'exact' — MessageStrip's own vocabulary (warn/err/note, no
// separate info) makes 'info' its informational tone (the same one the history viewing band uses),
// so 'exact' reuses it rather than widening a shared primitive for this one caller (§0.2/§3).
const fidelityTone = computed<'info' | 'warn'>(() =>
  wire.value?.fidelity === 'exact' ? 'info' : 'warn',
);

const maskingNote = computed(() => {
  const n = wire.value?.maskedSecrets ?? 0;
  if (n === 0) return '';
  return t('studio.http.raw.masked.description', {
    count: n,
    placeholder: '{{name}}',
    copyAsCurl: t('studio.api.copyAsCurl.title'),
  });
});

const elisionNote = computed(() =>
  wire.value?.requestBodyElided ? t('studio.http.raw.elided.description') : '',
);

// D7/D8: a live dump failure is this send's own — the only case left with no raw view at all,
// now that a stored entry always gets the reconstruction (F7: the request side is always stored).
const emptyLabel = computed(() => {
  if (!response.value || wire.value || showReconstructed.value) return '';
  return t('studio.http.raw.unavailable.empty');
});

const requestCaption = computed(() => {
  const e = viewingStored.value?.snapshot.entry;
  if (e) return t('studio.http.raw.requestCaption.label', { method: e.method, url: e.url });
  const r = rt.value;
  return r ? `→ ${t('studio.http.raw.requestPending.label')}` : '';
});

// D8: the unified document each editor renders — the live wire's own text, or the reconstruction,
// so the rest of the template (copy buttons, find-bar targets) reads one source regardless.
const requestText = computed(() => wire.value?.request ?? storedRequestText.value);
const responseText = computed(() => {
  if (wire.value && response.value) return `${wire.value.responseHead}\n${response.value.body}`;
  return storedResponseText.value;
});

function onCopyRequest(): void {
  void copyText(requestText.value);
}

function onCopyResponse(): void {
  void copyText(responseText.value);
}

// The two hosts and their docs, exposed for ResponsePane.vue's own find-bar targets
// (`targets: { doc, host }[]`) — `getDocs()` rather than exposed computed refs, since it's called
// from inside ResponsePane's own reactive computed (any reactive state it reads there is tracked
// exactly as if read directly, regardless of which component's function reads it).
const requestHostRef = ref<FindBarHost | null>(null);
const responseHostRef = ref<FindBarHost | null>(null);
defineExpose({
  requestHost: requestHostRef,
  responseHost: responseHostRef,
  getDocs: () => ({ request: requestText.value, response: responseText.value }),
});

// P107 I2-38: the wire and reconstructed branches rebuilt the same request/response section
// markup — one v-for over this instead. noteBefore/noteAfter are '' outside the `wire` branch
// (the reconstructed branch shows neither the elision nor the order note, exactly as before).
interface RawSection {
  key: 'request' | 'response';
  caption: string;
  captionTestid?: string;
  copyLabel: string;
  copyTestid: string;
  onCopy: () => void;
  doc: string;
  highlights?: (doc: string) => readonly RangeHighlight[];
  editorTestid: string;
  noteBefore: string;
  noteBeforeTestid?: string;
  noteAfter: string;
  noteAfterTestid?: string;
}

const rawSections = computed<RawSection[]>(() => [
  {
    key: 'request',
    caption: requestCaption.value,
    captionTestid: 'http-wire-request-caption',
    copyLabel: t('studio.http.raw.copyRequest.label'),
    copyTestid: 'http-wire-request-copy',
    onCopy: onCopyRequest,
    doc: requestText.value,
    highlights: props.requestHighlights,
    editorTestid: 'http-wire-request-editor',
    noteBefore: wire.value ? elisionNote.value : '',
    noteBeforeTestid: 'http-wire-elision-note',
    noteAfter: '',
  },
  {
    key: 'response',
    caption: '←',
    copyLabel: t('studio.http.raw.copyResponse.label'),
    copyTestid: 'http-wire-response-copy',
    onCopy: onCopyResponse,
    doc: responseText.value,
    highlights: props.responseHighlights,
    editorTestid: 'http-wire-response-editor',
    noteBefore: '',
    noteAfter: wire.value ? t('studio.http.raw.headerOrder.description') : '',
    noteAfterTestid: 'http-wire-order-note',
  },
]);

function setHostRef(key: RawSection['key'], instance: unknown): void {
  const host = instance as FindBarHost | null;
  if (key === 'request') requestHostRef.value = host;
  else responseHostRef.value = host;
}
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-1 overflow-auto p-1.5" data-testid="http-raw-pane">
    <template v-if="wire || showReconstructed">
      <template v-if="wire">
        <Alert :variant="fidelityTone" data-testid="http-wire-fidelity">
          <AlertDescription>{{ fidelityText }}</AlertDescription>
        </Alert>
        <Alert v-if="maskingNote" variant="info" data-testid="http-wire-masking-note">
          <AlertDescription>{{ maskingNote }}</AlertDescription>
        </Alert>
      </template>
      <!-- A stored entry's Raw pane, reconstructed from the four stage-1 fields the
           snapshot carries rather than a live `wire` dump (which is never stored). Not claimed
           as one of the three HttpWireFidelity values — a reconstruction is none of them — so
           this gets its own honest strip rather than a fourth, misleading fidelity value. -->
      <template v-else>
        <Alert variant="info" data-testid="http-raw-reconstructed">
          <AlertDescription>
            {{ t('studio.http.raw.reconstructed.description') }}
          </AlertDescription>
        </Alert>
        <Alert
          v-if="requestBodyStorageTruncated"
          variant="info"
          data-testid="http-history-request-truncated"
        >
          <AlertDescription>
            {{ t('studio.http.raw.bodyKept.description', { size: formatBytes(256 * 1024) }) }}
          </AlertDescription>
        </Alert>
        <Alert
          v-if="requestFieldsElided"
          variant="info"
          data-testid="http-history-request-fields-elided"
        >
          <AlertDescription>
            {{ t('studio.http.history.fieldsElided.description') }}
          </AlertDescription>
        </Alert>
      </template>

      <template v-for="section in rawSections" :key="section.key">
        <div class="flex min-h-52 flex-col gap-0.5">
          <div class="flex items-center gap-1">
            <span
              class="min-w-0 text-kira-sm text-subtle font-data"
              :data-testid="section.captionTestid"
            >
              <TruncatedText :text="section.caption" />
            </span>
            <span class="ml-auto" />
            <TooltipIconButton
              :icon="ICON.copy"
              :label="section.copyLabel"
              :data-testid="section.copyTestid"
              @click="section.onCopy"
            />
          </div>
          <Alert v-if="section.noteBefore" variant="info" :data-testid="section.noteBeforeTestid">
            <AlertDescription>{{ section.noteBefore }}</AlertDescription>
          </Alert>
          <div class="flex-1 min-h-52 overflow-hidden rounded-kira">
            <MonacoHost
              :ref="(el) => setHostRef(section.key, el)"
              :doc="section.doc"
              language="plain"
              readonly
              :range-highlights="section.highlights"
              :data-testid="section.editorTestid"
            />
          </div>
          <Alert v-if="section.noteAfter" variant="info" :data-testid="section.noteAfterTestid">
            <AlertDescription>{{ section.noteAfter }}</AlertDescription>
          </Alert>
        </div>
      </template>
    </template>

    <template v-else-if="emptyLabel">
      <EmptyState icon="file-binary" :title="emptyLabel" />
    </template>
    <EmptyState v-else icon="arrow-right" :title="t('studio.common.responseEmpty.empty')" />
  </div>
</template>
