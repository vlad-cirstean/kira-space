<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import OpLogCodeBlock from '@workbench/components/OpLogCodeBlock.vue';
import { computed } from 'vue';

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
</script>

<template>
  <ApprovalDetails>
    <template v-if="payload.script?.prepare">
      <template v-if="payload.script.prepare.changed.length">
        <dt>{{ t('studio.dbMcp.approval.api.scriptChanged.label') }}</dt>
        <dd data-testid="api-approval-script-changed">
          {{ payload.script.prepare.changed.join(', ') }}
        </dd>
      </template>
      <template v-if="payload.script.prepare.input">
        <dt>{{ t('studio.dbMcp.approval.api.scriptInput.label') }}</dt>
        <dd>
          <OpLogCodeBlock
            unboxed
            :text="payload.script.prepare.input"
            :language="payload.script.prepare.inputTruncated ? null : 'json'"
            data-testid="api-approval-script-input"
          />
          <span v-if="payload.script.prepare.inputTruncated">{{
            t('studio.common.truncated.status')
          }}</span>
        </dd>
      </template>
    </template>
    <dt>{{ t('studio.dbMcp.approval.detail.options.label') }}</dt>
    <dd data-testid="api-approval-options">{{ options }}</dd>
  </ApprovalDetails>
</template>
