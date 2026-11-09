<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
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
  </ApprovalDetails>
</template>
