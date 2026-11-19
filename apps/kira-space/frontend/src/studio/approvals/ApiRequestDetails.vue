<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import ApprovalPairs from '@workbench/approvals/ApprovalPairs.vue';
import { computed } from 'vue';
import { KEPT_MARK_CLASS, keptParts, keptRanges } from '../kept/keptHighlight';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiRequestPayloadSchema.parse(props.item.payload));
const options = computed(() =>
  t(
    payload.value.followRedirects
      ? 'studio.dbMcp.approval.api.redirectsOnTimeout.status'
      : 'studio.dbMcp.approval.api.redirectsOffTimeout.status',
    { seconds: payload.value.timeoutSeconds },
  ),
);

const headerParts = (row: number, part: 'name' | 'value') => {
  const text = payload.value.headers[row]?.[part] ?? '';
  const ranges = keptRanges(payload.value.kept, 'header', row, part);
  return ranges.length ? keptParts(text, ranges) : null;
};
</script>

<template>
  <ApprovalDetails>
    <template v-if="payload.headers.length">
      <dt>{{ t('studio.http.pane.headers.label') }}</dt>
      <dd>
        <ApprovalPairs
          :rows="payload.headers.map((h) => ({ name: h.name, value: h.value, secret: h.masked }))"
          :highlight="headerParts"
          :mark-class="KEPT_MARK_CLASS"
          testid="api-approval-headers"
        />
      </dd>
    </template>
    <template v-if="payload.variables.length">
      <dt>{{ t('common.module.variables.label') }}</dt>
      <dd>
        <ApprovalPairs
          :rows="payload.variables.map((v) => ({ name: v.name, value: v.value, secret: v.secret }))"
          testid="api-approval-variables"
        />
      </dd>
    </template>
    <dt>{{ t('studio.dbMcp.approval.detail.options.label') }}</dt>
    <dd data-testid="api-approval-options">{{ options }}</dd>
  </ApprovalDetails>
</template>
