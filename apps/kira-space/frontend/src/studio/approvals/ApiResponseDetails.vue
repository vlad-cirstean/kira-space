<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiResponsePayloadSchema } from '@shared/domain/dbmcp';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { formatBytes } from '@workbench/behaviour/format';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => apiResponsePayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails>
    <dt>{{ t('studio.http.raw.requestPending.label') }}</dt>
    <dd data-testid="api-review-request">
      <TruncatedText class="font-data" :text="`${payload.method} ${payload.url}`" />
    </dd>
    <dt>{{ t('studio.dbMcp.approval.api.responseResult.label') }}</dt>
    <dd data-testid="api-review-bytes">{{ formatBytes(payload.bytes) }}</dd>
    <template v-if="payload.scripted">
      <dt>{{ t('studio.dbMcp.approval.api.responseRaw.label') }}</dt>
      <dd data-testid="api-review-raw-bytes">{{ formatBytes(payload.rawBytes) }}</dd>
    </template>
  </ApprovalDetails>
</template>
