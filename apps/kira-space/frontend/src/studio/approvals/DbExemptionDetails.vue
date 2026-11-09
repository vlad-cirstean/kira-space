<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbExemptionPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => dbExemptionPayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails>
    <dt>{{ t('common.field.reason.label') }}</dt>
    <dd class="whitespace-pre-wrap" data-testid="db-mcp-exemption-reason">{{ payload.reason }}</dd>
  </ApprovalDetails>
</template>
