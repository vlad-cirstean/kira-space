<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { proofReviewPayloadSchema } from '@shared/domain/proof';
import { t } from '@theme/i18n';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => proofReviewPayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails v-if="payload.reason">
    <ApprovalDetailItem :label="t('common.field.reason.label')">
      <span class="whitespace-pre-wrap">{{ payload.reason }}</span>
    </ApprovalDetailItem>
  </ApprovalDetails>
</template>
