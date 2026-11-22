<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import ApprovalValues from '@workbench/approvals/ApprovalValues.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiRequestPayloadSchema.parse(props.item.payload));
const options = computed(() => [
  t(
    payload.value.followRedirects
      ? 'studio.dbMcp.approval.api.redirectsOn.status'
      : 'studio.dbMcp.approval.api.redirectsOff.status',
  ),
  t('studio.dbMcp.approval.api.timeout.status', { seconds: payload.value.timeoutSeconds }),
]);
</script>

<template>
  <ApprovalDetails>
    <template v-if="payload.script?.prepare">
      <ApprovalDetailItem
        v-if="payload.script.prepare.changed.length"
        :label="t('studio.dbMcp.approval.api.scriptChanged.label')"
        testid="api-approval-script-changed"
      >
        <ApprovalValues :values="payload.script.prepare.changed" data />
      </ApprovalDetailItem>
      <ApprovalDetailItem
        v-if="payload.script.prepare.input"
        :label="t('studio.dbMcp.approval.api.scriptInput.label')"
      >
        <ApprovalCode
          :text="payload.script.prepare.input"
          :language="payload.script.prepare.inputTruncated ? null : 'json'"
          data-testid="api-approval-script-input"
        />
        <span v-if="payload.script.prepare.inputTruncated">{{
          t('studio.common.truncated.status')
        }}</span>
      </ApprovalDetailItem>
    </template>
    <ApprovalDetailItem
      :label="t('studio.dbMcp.approval.detail.options.label')"
      testid="api-approval-options"
    >
      <ApprovalValues :values="options" />
    </ApprovalDetailItem>
  </ApprovalDetails>
</template>
