<script setup lang="ts">
import { Toggle } from '@theme/components/ui/toggle';
import { t } from '@theme/i18n';
import LogLines, { type LogLine } from '@workbench/components/LogLines.vue';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import { useCodeLineHeight } from './codeLine';
import { useReviewLogStore } from './reviewLogStore';

// A held log as terminal-style lines, verbatim. The shared log viewer virtualizes, so a long tail
// stays cheap; the toggles pick wrapping, timestamps and a line-number gutter.
const props = defineProps<{
  lines: readonly { ts?: string; text: string }[];
  testid: string;
}>();

const { wrap, timestamps, lineNumbers } = storeToRefs(useReviewLogStore());
const lineHeight = useCodeLineHeight();
const logLines = computed<LogLine[]>(() =>
  props.lines.map((l, id) => ({ id, text: l.text, stream: 'stdout', ts: l.ts })),
);
</script>

<template>
  <div class="flex flex-col gap-1.5">
    <div class="flex items-center gap-1">
      <Toggle
        v-model="wrap"
        size="kira"
        :data-testid="`${testid}-wrap`"
        :aria-label="t('common.logs.wrap.label')"
        >{{
          t('common.logs.wrap.label')
        }}</Toggle
      >
      <Toggle
        v-model="timestamps"
        size="kira"
        :data-testid="`${testid}-timestamps`"
        :aria-label="t('common.logs.timestamps.label')"
        >{{
          t('common.logs.timestamps.label')
        }}</Toggle
      >
      <Toggle
        v-model="lineNumbers"
        size="kira"
        :data-testid="`${testid}-line-numbers`"
        :aria-label="t('common.logs.lineNumbers.label')"
        >{{
          t('common.logs.lineNumbers.label')
        }}</Toggle
      >
    </div>
    <LogLines
      :lines="logLines"
      :testid="testid"
      max-height="18rem"
      :wrap="wrap"
      :show-timestamps="timestamps"
      :line-numbers="lineNumbers"
      levels
      :line-height="lineHeight"
    />
  </div>
</template>
