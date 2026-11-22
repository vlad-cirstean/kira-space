<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiResponsePayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { formatBytes } from '@workbench/behaviour/format';
import { computed } from 'vue';
import RequestLine from './RequestLine.vue';

const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => apiResponsePayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails>
    <ApprovalDetailItem :label="t('common.field.request.label')" testid="api-review-request">
      <RequestLine :method="payload.method" :url="payload.url" :kept="[]" />
    </ApprovalDetailItem>
    <ApprovalDetailItem :label="t('studio.http.diff.after.label')" testid="api-review-bytes">
      {{ formatBytes(payload.bytes) }}
    </ApprovalDetailItem>
    <ApprovalDetailItem
      v-if="payload.scripted"
      :label="t('studio.http.diff.before.label')"
      testid="api-review-raw-bytes"
    >
      {{ formatBytes(payload.rawBytes) }}
    </ApprovalDetailItem>
  </ApprovalDetails>
</template>
