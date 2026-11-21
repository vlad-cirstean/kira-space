<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { connectionKindSchema } from '@shared/domain/connection';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { formatBytes } from '@workbench/behaviour/format';
import OpLogCodeBlock from '@workbench/components/OpLogCodeBlock.vue';
import { computed } from 'vue';
import { consoleLanguageFor } from '../views/console/format';

const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => dbResponsePayloadSchema.parse(props.item.payload));
const language = computed(() =>
  consoleLanguageFor(connectionKindSchema.safeParse(props.item.target.kind).data),
);
</script>

<template>
  <ApprovalDetails>
    <dt>{{ t('studio.dbMcp.approval.detail.statement.label') }}</dt>
    <dd data-testid="db-mcp-preview-statement">
      <OpLogCodeBlock unboxed :text="payload.statement" :language="language" />
    </dd>
    <template v-if="payload.scripted">
      <dt>{{ t('common.agentGets.label') }}</dt>
      <dd data-testid="db-review-bytes">{{ formatBytes(payload.bytes) }}</dd>
      <dt>{{ t('studio.dbMcp.approval.query.responseRaw.label') }}</dt>
      <dd data-testid="db-review-raw-bytes">{{ formatBytes(payload.rawBytes) }}</dd>
    </template>
  </ApprovalDetails>
</template>
