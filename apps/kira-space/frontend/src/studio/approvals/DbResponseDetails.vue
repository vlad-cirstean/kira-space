<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { formatBytes } from '@workbench/behaviour/format';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => dbResponsePayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails>
    <dt>{{ t('studio.dbMcp.approval.detail.statement.label') }}</dt>
    <dd data-testid="db-mcp-preview-statement">
      <TruncatedText class="font-data" :text="payload.statement" />
    </dd>
    <template v-if="payload.scripted">
      <dt>{{ t('common.agentGets.label') }}</dt>
      <dd data-testid="db-review-bytes">{{ formatBytes(payload.bytes) }}</dd>
      <dt>{{ t('studio.dbMcp.approval.query.responseRaw.label') }}</dt>
      <dd data-testid="db-review-raw-bytes">{{ formatBytes(payload.rawBytes) }}</dd>
    </template>
  </ApprovalDetails>
</template>
