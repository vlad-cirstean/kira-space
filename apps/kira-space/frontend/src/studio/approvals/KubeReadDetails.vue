<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { kubeReadPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const p = computed(() => kubeReadPayloadSchema.parse(props.item.payload));
const options = computed(() => {
  const req = p.value.request;
  if (!req) return '';
  if (p.value.operation === 'list') {
    return t('studio.dbMcp.approval.kube.rows.status', { count: req.maxRows });
  }
  if (p.value.operation !== 'logs') return '';
  return [
    t('studio.dbMcp.approval.kube.tail.status', { tail: req.tail }),
    req.previous && t('studio.dbMcp.approval.kube.previous.label'),
    req.timestamps && t('common.logs.timestamps.label'),
  ]
    .filter(Boolean)
    .join(' · ');
});
</script>

<template>
  <ApprovalDetails>
    <dt>{{ t('common.field.command.label') }}</dt>
    <dd class="break-all font-data" data-testid="kube-approval-detail-command">{{
      p.statement
    }}</dd>
    <dt>{{ t('studio.dbMcp.approval.kube.server.label') }}</dt>
    <dd class="break-all font-data" data-testid="kube-approval-detail-server">{{ p.server }}</dd>
    <template v-if="options">
      <dt>{{ t('studio.dbMcp.approval.detail.options.label') }}</dt>
      <dd data-testid="kube-approval-detail-options">{{ options }}</dd>
    </template>
    <template v-if="p.changed.length">
      <dt>{{ t('studio.dbMcp.approval.api.scriptChanged.label') }}</dt>
      <dd data-testid="kube-approval-script-changed">{{ p.changed.join(', ') }}</dd>
    </template>
  </ApprovalDetails>
</template>
