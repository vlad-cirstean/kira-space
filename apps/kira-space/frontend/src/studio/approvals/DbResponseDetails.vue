<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { connectionKindSchema } from '@shared/domain/connection';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { formatBytes } from '@workbench/behaviour/format';
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
    <ApprovalDetailItem
      :label="t('studio.dbMcp.approval.detail.statement.label')"
      testid="db-mcp-preview-statement"
    >
      <ApprovalCode :text="payload.statement" :language="language" />
    </ApprovalDetailItem>
    <template v-if="payload.scripted">
      <ApprovalDetailItem :label="t('studio.http.diff.after.label')" testid="db-review-bytes">
        {{ formatBytes(payload.bytes) }}
      </ApprovalDetailItem>
      <ApprovalDetailItem :label="t('studio.http.diff.before.label')" testid="db-review-raw-bytes">
        {{ formatBytes(payload.rawBytes) }}
      </ApprovalDetailItem>
    </template>
  </ApprovalDetails>
</template>
