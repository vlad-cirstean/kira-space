<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { proofReviewPayloadSchema } from '@shared/domain/proof';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => proofReviewPayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails v-if="payload.reason">
    <dt>{{ t('common.field.reason.label') }}</dt>
    <dd class="whitespace-pre-wrap">{{ payload.reason }}</dd>
  </ApprovalDetails>
</template>
