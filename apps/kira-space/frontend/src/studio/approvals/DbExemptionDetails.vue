<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbExemptionPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => dbExemptionPayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails>
    <ApprovalDetailItem :label="t('common.field.reason.label')" testid="db-mcp-exemption-reason">
      <span class="whitespace-pre-wrap">{{ payload.reason }}</span>
    </ApprovalDetailItem>
  </ApprovalDetails>
</template>
