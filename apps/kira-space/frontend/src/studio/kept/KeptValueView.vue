<script setup lang="ts">
import MaskedValue from '@theme/components/MaskedValue.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { FieldError } from '@theme/components/ui/field';
import ValueRevealButton from '@theme/components/ValueRevealButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import CopyIconButton from '@workbench/components/CopyIconButton.vue';
import { computed } from 'vue';
import { beautifyJson, indentJson } from '../beautify';
import { control } from '../bridge/control';
import { useKeptReveal } from './useKeptReveal';

// A temporary value on one line: the preview in the data font, cut by the cell with the whole text in a
// tooltip. A secret shows dots; Reveal asks the OS gate, shows the value for a minute and only then offers
// Copy. The agent never gets any of this. A stored value whose preview the backend cut offers `open`, which
// the row uses to show the whole value under the line. `staged` is a value not stored yet (a held result's
// "Will set"): only its preview exists, so nothing offers the whole value. A staged secret has `stagedReveal`:
// the held result's request and the name, read behind the same gate.
const props = defineProps<{
  entry: { id: string; type: string; secret: boolean; preview: string; previewTruncated: boolean };
  staged?: boolean;
  stagedReveal?: { requestId: string; name: string };
}>();
const open = defineModel<boolean>('open', { default: false });

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

// One line: a string's first line, an object or list compacted; a cut preview is not valid JSON, so its
// first line shows as it is with `…`.
const line = computed(() => {
  const { preview, previewTruncated } = props.entry;
  const p = previewTruncated ? { ok: false as const } : parsed(preview);
  if (!p.ok) {
    const [first = '', ...rest] = preview.split('\n');
    return previewTruncated || rest.length ? `${first}…` : first;
  }
  // Compact from the source text: re-stringifying a parsed number would round a big integer.
  if (typeof p.value !== 'string') {
    const r = beautifyJson(preview, 'compact');
    return r.ok ? r.text : preview;
  }
  const [first = '', ...rest] = p.value.split('\n');
  return rest.length ? `${first}…` : first;
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
    <div class="flex min-w-0 items-center gap-1">
      <template v-if="entry.secret">
        <span
          v-if="secretReveal.value.value !== null"
          class="min-w-0 flex-1 font-data text-kira-md"
          data-testid="kept-value-revealed"
          ><TruncatedText :text="secretReveal.value.value" /></span
        >
        <MaskedValue v-else class="flex-1 font-data" data-testid="kept-value-secret" />
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
      </template>
      <template v-else>
        <span class="min-w-0 flex-1 font-data text-kira-md" data-testid="kept-value-preview"
          ><TruncatedText :text="line" /></span
        >
        <CopyIconButton
          v-if="!staged || !entry.previewTruncated"
          :text="valueText"
          :label="t('common.action.copyValue.label')"
          :message="t('common.copy.copiedValue.announce')"
          data-testid="kept-value-copy"
        />
        <TooltipIconButton
          v-if="entry.previewTruncated && !staged"
          :icon="ICON.expand"
          :label="t('common.action.showAll.label')"
          :pressed="open"
          data-testid="kept-value-showall"
          @click="open = !open"
        />
      </template>
    </div>
    <FieldError v-if="secretReveal.error.value" data-testid="kept-value-reveal-error">{{
      secretReveal.error.value
    }}</FieldError>
  </div>
</template>
