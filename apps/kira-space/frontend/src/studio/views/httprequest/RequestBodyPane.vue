<script setup lang="ts">
import {
  BODY_MODE_OPTIONS,
  CODE_LANGUAGE_OPTIONS,
  contentTypeCaption,
  editorLanguageForCode,
  type HttpBodySelection,
  userContentTypeHeader,
} from '@kira/api-core';
import type { HttpCodeLanguage } from '@shared/domain/http';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { NativeSelect } from '@theme/components/ui/native-select';
import { type MessageKey, t } from '@theme/i18n';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import type { RangeHighlight } from '@workbench/editor/ranges';
import { computed, ref } from 'vue';
import {
  type VariableSupport,
  variableCompletionSource,
  variableHoverSource,
} from '../../api/state/variableCompletion';
import { patchHttpRequestTabState } from '../../api/tabs';
import MonacoHost from '../../editor/MonacoHost.vue';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import { INLINE_CHARS } from '../../workers/parse/client';
import { useParseWorker } from '../../workers/parse/useParseWorker';
import { beautifyFor, canBeautify } from '../shared/celleditor/formats';
import { reasonText } from '../shared/reasonText';
import BinaryBodyPicker from './BinaryBodyPicker.vue';
import FormDataTable from './FormDataTable.vue';
import UrlEncodedTable from './UrlEncodedTable.vue';

// The mode selector, the code-language select, the auto-Content-Type caption and the per-mode
// editor host all live here now so HttpRequestView.vue stays a layout shell. `raw` is a plain-text
// buffer with no sub-selector; `code` keeps the language selector raw used to carry, narrowed to
// its four syntax-highlighted languages.
const props = defineProps<{
  tab: HttpRequestTabRecord;
  /** P15b D4: HttpRequestView.vue's own variableSupport(...) — rangeHighlights colours both
   *  editable editor hosts below (raw and code/JSON) with `{{variable}}` references exactly like
   *  the URL/header fields (N3); the rest is forwarded to the urlencoded/form-data value cells. */
  variables?: VariableSupport;
  /** P16 D13: HttpRequestView.vue's own #toolbar-2 filter box, forwarded to whichever body mode
   *  is a row table (urlencoded, form-data) — a no-op for the raw/code/binary modes below. */
  filterQuery?: string;
  /** P22b D7: HttpRequestView.vue's own persisted description-column toggle, forwarded the same
   *  way — a no-op for the raw/code/binary modes below. */
  showDescriptions?: boolean;
  /** The hoisted find bar's own ranges for whichever of the two text editors is showing.
   *  This pane's `rangeHighlights` compartment was already spoken for by the {{variable}}
   *  colouring (the response pane's own comment names that as the reason the find bar could
   *  never reach the request body), so the two sources are merged below rather than one replacing
   *  the other. */
  findHighlights?: (doc: string) => readonly RangeHighlight[];
}>();

/** Variable ranges first, find ranges second — a match's class wins on overlap, which is
 *  what a user searching for text that happens to sit inside a {{reference}} expects to see.
 *  rangeHighlightPlugin sorts and validates whatever it is handed, so no ordering guarantee is
 *  required of this concatenation beyond that intent. Identity changes whenever either source
 *  does, which is what makes MonacoHost's own watch repaint. */
const bodyHighlights = computed<((doc: string) => readonly RangeHighlight[]) | undefined>(() => {
  const vars = props.variables?.rangeHighlights;
  const find = props.findHighlights;
  if (!find) return vars;
  if (!vars) return find;
  return (doc: string) => [...vars(doc), ...find(doc)];
});

/** The two editable text buffers this pane can show, in the order the find bar numbers them —
 *  exactly one is mounted at a time, so this is a one-element list or empty. */
const findableDoc = computed<string | null>(() => {
  if (props.tab.state.bodyMode === 'raw') return props.tab.state.body;
  if (props.tab.state.bodyMode === 'code') return props.tab.state.code;
  return null;
});
// The mounted editor host, so the find bar can scroll a match into view — exactly one of the two
// is ever mounted, so whichever ref is non-null is the live one.
const rawHostRef = ref<{ scrollRangeIntoView(from: number, to: number): void } | null>(null);
const codeHostRef = ref<{ scrollRangeIntoView(from: number, to: number): void } | null>(null);
const findHost = computed(() => rawHostRef.value ?? codeHostRef.value);
defineExpose({ findableDoc, findHost });

// P15 D6: JSON is a UI-level segment over the same `bodyMode`/`codeLanguage` storage — no schema,
// wire or Go change (§5 of the plan spells out why: the entire delta below is presentation).
const selection = computed<HttpBodySelection>(() =>
  props.tab.state.bodyMode === 'code' && props.tab.state.codeLanguage === 'json'
    ? 'json'
    : props.tab.state.bodyMode,
);

function setSelection(next: HttpBodySelection): void {
  if (next === 'json') {
    patchHttpRequestTabState(props.tab.id, { bodyMode: 'code', codeLanguage: 'json' });
    return;
  }
  if (next === 'code' && props.tab.state.codeLanguage === 'json') {
    // Otherwise `selection` above would snap straight back to 'json' and this segment would be
    // unclickable — Code always means a non-JSON language now that JSON has its own segment.
    patchHttpRequestTabState(props.tab.id, { bodyMode: 'code', codeLanguage: 'javascript' });
    return;
  }
  patchHttpRequestTabState(props.tab.id, { bodyMode: next });
}

function onCodeLanguageChange(value: unknown): void {
  const codeLanguage = String(value) as HttpCodeLanguage;
  patchHttpRequestTabState(props.tab.id, { codeLanguage });
  beautifyError.value = null;
}

function onRawChange(text: string): void {
  patchHttpRequestTabState(props.tab.id, { body: text });
}

function onCodeChange(text: string): void {
  patchHttpRequestTabState(props.tab.id, { code: text });
  beautifyError.value = null;
}

// D9: Beautify offered exactly where formats.ts's own canBeautify says a lossless reformatter
// exists (json/xml) — reused, not re-derived (§3: no new beautify logic).
const beautifyFormat = computed<'json' | 'xml' | null>(() => {
  const lang = props.tab.state.codeLanguage;
  if (lang !== 'json' && lang !== 'xml') return null;
  return canBeautify(lang) ? lang : null;
});

const parse = useParseWorker();
const beautifyError = ref<string | null>(null);
async function onBeautifyBody(): Promise<void> {
  const fmt = beautifyFormat.value;
  if (!fmt) return;
  const source = props.tab.state.code;
  const result =
    source.length <= INLINE_CHARS
      ? beautifyFor(fmt, source, 'indented')
      : await parse.run(`${fmt}.beautify`, { text: source, mode: 'indented' }).catch(() => null);
  if (!result) return; // the view unmounted mid-run
  // The buffer moved on while the worker ran: the result describes older text.
  if (props.tab.state.code !== source) return;
  if (result.ok) {
    patchHttpRequestTabState(props.tab.id, { code: result.text });
    beautifyError.value = null;
  } else {
    beautifyError.value = result.reason
      ? reasonText(result.reason)
      : t('studio.http.body.beautifyFailed.error');
  }
}

const editorLanguage = computed(() => editorLanguageForCode(props.tab.state.codeLanguage));

// D9: the honest alternative to Postman's greyed "hidden headers" list — states exactly what D7's
// Content-Type precedence will do, without ever injecting a synthetic row into state.headers.
const caption = computed(() =>
  contentTypeCaption(
    props.tab.state.bodyMode,
    props.tab.state.codeLanguage,
    userContentTypeHeader(props.tab.state.headers),
  ),
);

const BODY_MODE_LABEL = {
  none: 'common.state.none.option',
  formdata: 'studio.http.bodyMode.formdata.label',
  urlencoded: 'studio.http.bodyMode.urlencoded.label',
  raw: 'common.raw.label',
  json: 'studio.copy.format.json.label',
  code: 'common.code.label',
  file: 'studio.http.bodyMode.file.label',
} as const satisfies Record<HttpBodySelection, MessageKey>;
const BODY_MODE_TOOLTIP = {
  none: 'studio.http.bodyMode.none.tooltip',
  formdata: 'studio.http.bodyMode.formdata.tooltip',
  urlencoded: 'studio.http.bodyMode.urlencoded.tooltip',
  raw: 'common.plainText.label',
  json: 'studio.http.bodyMode.json.tooltip',
  code: 'studio.http.bodyMode.code.tooltip',
  file: 'studio.http.bodyMode.file.tooltip',
} as const satisfies Record<HttpBodySelection, MessageKey>;
const BODY_MODE_ITEMS = computed(() =>
  BODY_MODE_OPTIONS.map((opt) => ({
    value: opt.value,
    label: t(BODY_MODE_LABEL[opt.value]),
    testid: opt.testid,
    tooltip: t(BODY_MODE_TOOLTIP[opt.value]),
  })),
);
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col">
    <ViewToolbar class="overflow-x-auto">
      <SecondaryTabs
        variant="segmented"
        :model-value="selection"
        :items="BODY_MODE_ITEMS"
        data-testid="http-body-mode"
        @update:model-value="(v) => setSelection(v as HttpBodySelection)"
      />
      <NativeSelect
        v-if="tab.state.bodyMode === 'code' && tab.state.codeLanguage !== 'json'"
        variant="bordered"
        data-testid="http-body-code-language"
        :model-value="tab.state.codeLanguage"
        @update:model-value="onCodeLanguageChange"
      >
        <option v-for="opt in CODE_LANGUAGE_OPTIONS" :key="opt.value" :value="opt.value">
          {{ t(opt.label.key) }}
        </option>
      </NativeSelect>
      <span class="ml-auto" />
      <TooltipIconButton
        v-if="tab.state.bodyMode === 'code' && beautifyFormat"
        icon="expand-all"
        :label="t('studio.api.common.beautify.label')"
        data-testid="http-body-beautify"
        @click="onBeautifyBody"
      />
    </ViewToolbar>

    <div
      v-if="caption"
      class="text-kira-sm text-subtle px-1.5 pt-0 pb-1"
      data-testid="http-body-content-type-caption"
    >
      {{ caption }}
    </div>

    <Alert strip v-if="beautifyError" variant="error" data-testid="http-body-beautify-error">
      <AlertDescription>{{ beautifyError }}</AlertDescription>
    </Alert>

    <MonacoHost
      v-if="tab.state.bodyMode === 'raw'"
      :doc="tab.state.body"
      ref="rawHostRef"
      language="plain"
      :range-highlights="bodyHighlights"
      :hover-source="variables && variableHoverSource(variables.hoverInfo)"
      :autocomplete="!!variables"
      :completion-sources="variables && [variableCompletionSource(variables.candidates)]"
      auto-close-brackets
      @update:doc="onRawChange"
    />
    <MonacoHost
      v-else-if="tab.state.bodyMode === 'code'"
      :doc="tab.state.code"
      ref="codeHostRef"
      :language="editorLanguage"
      :range-highlights="bodyHighlights"
      :hover-source="variables && variableHoverSource(variables.hoverInfo)"
      :autocomplete="!!variables"
      :completion-sources="variables && [variableCompletionSource(variables.candidates)]"
      auto-close-brackets
      @update:doc="onCodeChange"
    />
    <UrlEncodedTable
      v-else-if="tab.state.bodyMode === 'urlencoded'"
      :tab="tab"
      :variables="variables"
      :filter-query="filterQuery"
      :show-descriptions="showDescriptions"
    />
    <FormDataTable
      v-else-if="tab.state.bodyMode === 'formdata'"
      :tab="tab"
      :variables="variables"
      :filter-query="filterQuery"
      :show-descriptions="showDescriptions"
    />
    <BinaryBodyPicker v-else-if="tab.state.bodyMode === 'file'" :tab="tab" />
  </div>
</template>
