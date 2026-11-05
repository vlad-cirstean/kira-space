<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { EditBuffer } from './useEditBuffer';

// The one visual form of the three features an edit buffer offers — the `modified` chip, the
// byte badge, Beautify/Minify and Revert — rendered identically by the cell editor's header and
// by the document row's edit-action row. `testidPrefix` keeps each mount's testids
// distinct without either caller inventing its own copy of this row.
const props = withDefaults(
  defineProps<{
    buffer: EditBuffer;
    revertTitle?: string;
    testidPrefix: string;
    /** Mongo's shell-literal document editor has no real use for a minified constructor-call
     *  body (`ObjectId(...)`, `ISODate(...)`) the way the cell editor's raw JSON/XML does — the
     *  Minify action is hidden there rather than offering a control that doesn't serve a purpose. */
    showCompact?: boolean;
    /** P42 D31: the cell editor's own status badge already carries a byte figure (among truncation/
     *  decoded-reading/beautify-failure notes) — this row's badge would be the same number shown
     *  twice. The document editor has no such badge, so its two mounts keep the default. */
    showBytes?: boolean;
  }>(),
  { revertTitle: undefined, showCompact: true, showBytes: true },
);

const beautifyIndentedTitle = computed<string>(() =>
  t(
    props.buffer.canBeautify.value
      ? 'studio.editBuffer.beautify.tooltip'
      : 'studio.editBuffer.formatUnsupported.tooltip',
  ),
);
const beautifyCompactTitle = computed<string>(() =>
  t(
    props.buffer.canBeautify.value
      ? 'studio.editBuffer.minify.tooltip'
      : 'studio.editBuffer.formatUnsupported.tooltip',
  ),
);
const resetTitle = computed<string>(
  () =>
    props.revertTitle ??
    (props.buffer.isDirty.value
      ? t('studio.editBuffer.revert.tooltip')
      : t('studio.editBuffer.reverted.tooltip')),
);
</script>

<template>
  <span class="flex items-center shrink-0 gap-1.5">
    <Badge v-if="buffer.isDirty.value" variant="warn" :data-testid="`${testidPrefix}-modified`">
      {{ t('studio.editBuffer.modified.status') }}
    </Badge>
    <Badge data v-if="showBytes" :data-testid="`${testidPrefix}-byte-badge`">{{
      buffer.byteLabel.value
    }}</Badge>
    <TooltipIconButton
      icon="expand-all"
      :label="beautifyIndentedTitle"
      :aria-label="t('studio.api.common.beautify.label')"
      disabled-trigger
      :pressed="buffer.formatted.value === 'indented'"
      :data-testid="`${testidPrefix}-beautify-indented`"
      :disabled="!buffer.canBeautify.value"
      @click="buffer.applyBeautify('indented')"
    />
    <TooltipIconButton
      v-if="showCompact"
      icon="collapse-all"
      :label="beautifyCompactTitle"
      :aria-label="t('studio.editBuffer.minify.label')"
      disabled-trigger
      :pressed="buffer.formatted.value === 'compact'"
      :data-testid="`${testidPrefix}-beautify-compact`"
      :disabled="!buffer.canBeautify.value"
      @click="buffer.applyBeautify('compact')"
    />
    <TooltipIconButton
      icon="discard"
      :label="resetTitle"
      :aria-label="t('common.revert.label')"
      disabled-trigger
      :data-testid="`${testidPrefix}-beautify-reset`"
      :disabled="!buffer.isDirty.value"
      @click="buffer.reset()"
    />
  </span>
</template>
