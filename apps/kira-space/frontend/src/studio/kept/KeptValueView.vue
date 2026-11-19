<script setup lang="ts">
import Disclosure from '@theme/components/Disclosure.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref } from 'vue';
import FlushEditor from '../approvals/FlushEditor.vue';
import { prettyJson } from '../approvals/prettyJson';
import { control } from '../bridge/control';
import { useKeptValue } from './keptQueries';

// A kept value as the user sees it: a preview in the data font, the whole value on demand. A secret
// shows dots and offers no copy. The agent never gets any of this. `staged` is a value not stored yet
// (a held result's "Will keep"): only its preview exists, so nothing offers the whole value.
const props = defineProps<{
  entry: { id: string; type: string; secret: boolean; preview: string; previewTruncated: boolean };
  staged?: boolean;
}>();

const PREVIEW_LINES = 20;

function parsed(json: string): { ok: true; value: unknown } | { ok: false } {
  try {
    return { ok: true, value: JSON.parse(json) };
  } catch {
    return { ok: false };
  }
}

/** Strings raw, other JSON pretty-printed; the text of a whole value, as copied. A value that
 *  re-indenting would change (a big integer) stays as it is. */
function plainText(json: string): string {
  const p = parsed(json);
  if (!p.ok) return json;
  return typeof p.value === 'string' ? p.value : (prettyJson(json) ?? json);
}

const lines = computed(() => plainText(props.entry.preview).split('\n'));
const cut = computed(() => props.entry.previewTruncated || lines.value.length > PREVIEW_LINES);
// A cut preview is not valid JSON: show its text as it is.
const preview = computed(() => {
  const all = props.entry.previewTruncated ? props.entry.preview.split('\n') : lines.value;
  return all.slice(0, PREVIEW_LINES).join('\n') + (cut.value ? '…' : '');
});

const open = ref(false);
const full = useKeptValue(
  () => props.entry.id,
  () => open.value && cut.value,
);
const fullDoc = computed(() => {
  const json = full.data.value?.json;
  if (json === undefined) return '';
  return prettyJson(json) ?? json;
});

const { copy, copied } = useCopy();
async function copyValue() {
  const text = props.entry.previewTruncated
    ? plainText((await control.keptValue(props.entry.id)).json)
    : plainText(props.entry.preview);
  await copy(text, { message: t('common.copy.copiedValue.announce') });
}
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1" data-testid="kept-value">
    <span v-if="entry.secret" class="font-data text-subtle" data-testid="kept-value-secret"
      >••••</span
    >
    <template v-else>
      <div class="flex min-w-0 items-start gap-1">
        <div
          class="min-w-0 flex-1 whitespace-pre-wrap break-all font-data text-kira-md"
          data-testid="kept-value-preview"
          v-text="preview"
        />
        <TooltipIconButton
          v-if="!staged || !entry.previewTruncated"
          :icon="copied ? 'check' : 'copy'"
          :label="t('common.action.copyValue.label')"
          data-testid="kept-value-copy"
          @click="copyValue"
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
