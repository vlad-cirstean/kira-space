<script setup lang="ts">
import type { AgentApiResult } from '@shared/domain/dbmcp';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { type StatusTone, TONE_BADGE } from '@theme/lib/statusTone';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import ApprovalPairs from '@workbench/approvals/ApprovalPairs.vue';
import { formatBytes } from '@workbench/behaviour/format';
import { computed } from 'vue';
import FlushEditor from './FlushEditor.vue';
import { prettyJson } from './prettyJson';

// A response as the agent's request got it, masked: status, headers, body. The body scrolls inside the
// card, so a 2 MiB body does not grow the dialog.
const props = defineProps<{ response: AgentApiResult; planUrl: string; testid: string }>();

const tone = computed<StatusTone>(() => {
  const s = props.response.status;
  if (s >= 500) return 'error';
  if (s >= 400) return 'warn';
  if (s >= 200 && s < 300) return 'ok';
  return 'neutral';
});
const statusText = computed(() =>
  [props.response.status, props.response.statusText].filter(Boolean).join(' '),
);
const finalUrl = computed(() =>
  props.response.finalUrl && props.response.finalUrl !== props.planUrl
    ? props.response.finalUrl
    : '',
);
const json = computed(() => {
  const r = props.response;
  if (r.binary || r.truncated || !/json/i.test(r.contentType)) return null;
  return prettyJson(r.body);
});
</script>

<template>
  <div class="flex flex-col gap-1.5" :data-testid="testid">
    <div class="flex items-center gap-2">
      <Badge :variant="TONE_BADGE[tone]" :data-testid="`${testid}-status`">{{ statusText }}</Badge>
    </div>
    <ApprovalCode v-if="finalUrl" :data-testid="`${testid}-final-url`">{{ finalUrl }}</ApprovalCode>
    <ApprovalPairs
      v-if="response.redirects.length"
      :rows="response.redirects.map((r) => ({ name: String(r.status), value: r.url }))"
      :testid="`${testid}-redirects`"
    />
    <ApprovalPairs
      v-if="response.headers.length"
      :rows="response.headers"
      :testid="`${testid}-headers`"
    />
    <div class="max-h-80 overflow-auto" :data-testid="`${testid}-body`">
      <span v-if="response.binary" class="text-kira-sm text-subtle">{{
        t('studio.http.response.binary.status', { size: formatBytes(response.bodyBytes) })
      }}</span>
      <FlushEditor v-else-if="json !== null" :doc="json" language="json" />
      <ApprovalCode v-else-if="response.body">{{ response.body }}</ApprovalCode>
    </div>
    <span v-if="response.truncated" class="text-kira-sm text-subtle">{{
      t('studio.http.response.truncated.description', { size: formatBytes(response.bodyBytes) })
    }}</span>
  </div>
</template>
