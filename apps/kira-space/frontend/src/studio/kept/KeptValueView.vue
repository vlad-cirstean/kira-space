<script setup lang="ts">
import Disclosure from '@theme/components/Disclosure.vue';
import MaskedValue from '@theme/components/MaskedValue.vue';
import { FieldError } from '@theme/components/ui/field';
import ValueRevealButton from '@theme/components/ValueRevealButton.vue';
import { t } from '@theme/i18n';
import CopyIconButton from '@workbench/components/CopyIconButton.vue';
import OpLogCodeBlock from '@workbench/components/OpLogCodeBlock.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref } from 'vue';
import FlushEditor from '../approvals/FlushEditor.vue';
import { indentJson } from '../beautify';
import { control } from '../bridge/control';
import { useKeptValue } from './keptQueries';
import { useKeptReveal } from './useKeptReveal';

// A kept value as the user sees it: a preview in the data font, the whole value on demand. A secret
// shows dots; Reveal asks the OS gate, shows the value for a minute and only then offers Copy. The
// agent never gets any of this. `staged` is a value not stored yet
// (a held result's "Will keep"): only its preview exists, so nothing offers the whole value. A staged
// secret has `stagedReveal`: the held result's request and the name, read behind the same gate.
const props = defineProps<{
  entry: { id: string; type: string; secret: boolean; preview: string; previewTruncated: boolean };
  staged?: boolean;
  stagedReveal?: { requestId: string; name: string };
}>();

const PREVIEW_LINES = 20;

function parsed(json: string): { ok: true; value: unknown } | { ok: false } {
  try {
    return { ok: true, value: JSON.parse(json) };
  } catch {
    return { ok: false };
  }
}

/** Strings raw, other JSON pretty-printed; the text of a whole value, as copied. */
function plainText(json: string): string {
  const p = parsed(json);
  if (!p.ok) return json;
  return typeof p.value === 'string' ? p.value : (indentJson(json) ?? json);
}

const lines = computed(() => plainText(props.entry.preview).split('\n'));
const cut = computed(() => props.entry.previewTruncated || lines.value.length > PREVIEW_LINES);
// A cut preview is not valid JSON: show its text as it is.
const preview = computed(() => {
  const all = props.entry.previewTruncated ? props.entry.preview.split('\n') : lines.value;
  return all.slice(0, PREVIEW_LINES).join('\n') + (cut.value ? '…' : '');
});

// A whole JSON object or array preview is syntax-coloured; a string or cut preview stays plain.
const previewIsJson = computed(() => {
  if (props.entry.previewTruncated || cut.value) return false;
  const p = parsed(props.entry.preview);
  return p.ok && typeof p.value === 'object' && p.value !== null;
});

const open = ref(false);
const full = useKeptValue(
  () => props.entry.id,
  () => open.value && cut.value,
);
const fullDoc = computed(() => {
  const json = full.data.value?.json;
  if (json === undefined) return '';
  return indentJson(json) ?? json;
});

const secretReveal = useKeptReveal((confirmed) =>
  props.stagedReveal
    ? control.keptRevealStaged(props.stagedReveal.requestId, props.stagedReveal.name, confirmed)
    : control.keptReveal(props.entry.id, confirmed),
);

async function valueText(): Promise<string> {
  return props.entry.previewTruncated
    ? plainText((await control.keptValue(props.entry.id)).json)
    : plainText(props.entry.preview);
}
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1" data-testid="kept-value">
    <div v-if="entry.secret" class="flex min-w-0 items-start gap-1">
      <div
        v-if="secretReveal.value.value !== null"
        class="min-w-0 flex-1 whitespace-pre-wrap break-all font-data text-kira-md"
        data-testid="kept-value-revealed"
        v-text="secretReveal.value.value"
      />
      <MaskedValue v-else class="font-data" data-testid="kept-value-secret" />
      <template v-if="!staged || stagedReveal">
        <CopyIconButton
          v-if="secretReveal.value.value !== null"
          :text="secretReveal.value.value"
          :label="t('common.action.copyValue.label')"
          :message="t('common.copy.copiedValue.announce')"
          data-testid="kept-value-copy"
        />
        <ValueRevealButton
          :shown="secretReveal.value.value !== null"
          :disabled="secretReveal.busy.value"
          data-testid="kept-value-reveal"
          @click="secretReveal.value.value !== null ? secretReveal.hide() : secretReveal.reveal()"
        />
      </template>
    </div>
    <FieldError v-if="secretReveal.error.value" data-testid="kept-value-reveal-error">{{
      secretReveal.error.value
    }}</FieldError>
    <template v-if="!entry.secret">
      <div class="flex min-w-0 items-start gap-1">
        <OpLogCodeBlock
          unboxed
          class="min-w-0 flex-1"
          :text="preview"
          :language="previewIsJson ? 'json' : null"
          data-testid="kept-value-preview"
        />
        <CopyIconButton
          v-if="!staged || !entry.previewTruncated"
          :text="valueText"
          :label="t('common.action.copyValue.label')"
          :message="t('common.copy.copiedValue.announce')"
          data-testid="kept-value-copy"
        />
      </div>
      <Disclosure
        v-if="cut && !staged"
        v-model:open="open"
        :label="t('common.action.showAll.label')"
        meta
      >
        <QueryState :query="full" variant="inline">
          <FlushEditor :doc="fullDoc" language="json" data-testid="kept-value-full" />
        </QueryState>
      </Disclosure>
    </template>
  </div>
</template>
