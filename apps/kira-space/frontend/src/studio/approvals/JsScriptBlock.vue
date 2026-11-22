<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { computedAsync } from '@vueuse/core';
import { computed } from 'vue';
import { scriptVarRanges } from '../kept/keptHighlight';
import FlushEditor from './FlushEditor.vue';
import { formatScript } from './formatScript';

// A script the agent attached, read-only. Shown formatted (display only: the script runs as written) with
// every plain.x and secret.x token marked.
const props = defineProps<{ label: string; source: string; testid: string }>();

const shown = computedAsync(() => formatScript(props.source), {
  text: props.source,
  formatted: false,
});
const doc = computed(() => shown.value.text);
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="label">
      <TextTooltip
        v-if="shown.formatted"
        :text="t('studio.dbMcp.approval.script.formatted.tooltip')"
      >
        <Badge :data-testid="`${testid}-formatted`">{{
          t('studio.dbMcp.approval.formatted.option')
        }}</Badge>
      </TextTooltip>
    </SectionHeading>
    <FlushEditor :doc="doc" language="javascript" :range-highlights="scriptVarRanges" />
  </section>
</template>
