<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { kubeReadPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import ApprovalValues from '@workbench/approvals/ApprovalValues.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const p = computed(() => kubeReadPayloadSchema.parse(props.item.payload));
const options = computed((): string[] => {
  const req = p.value.request;
  if (!req) return [];
  if (p.value.operation === 'list') {
    return [t('studio.dbMcp.approval.kube.rows.status', { count: req.maxRows })];
  }
  if (p.value.operation !== 'logs') return [];
  return [
    t('studio.dbMcp.approval.kube.tail.status', { tail: req.tail }),
    req.previous && t('studio.dbMcp.approval.kube.previous.label'),
    req.timestamps && t('common.logs.timestamps.label'),
  ].filter((v): v is string => Boolean(v));
});
</script>

<template>
  <ApprovalDetails>
    <ApprovalDetailItem
      :label="t('common.field.command.label')"
      testid="kube-approval-detail-command"
    >
      <ApprovalCode>{{ p.statement }}</ApprovalCode>
    </ApprovalDetailItem>
    <ApprovalDetailItem
      :label="t('studio.dbMcp.approval.kube.server.label')"
      testid="kube-approval-detail-server"
    >
      <span class="break-all font-data">{{ p.server }}</span>
    </ApprovalDetailItem>
    <ApprovalDetailItem
      v-if="options.length"
      :label="t('studio.dbMcp.approval.detail.options.label')"
      testid="kube-approval-detail-options"
    >
      <ApprovalValues :values="options" />
    </ApprovalDetailItem>
    <ApprovalDetailItem
      v-if="p.changed.length"
      :label="t('studio.dbMcp.approval.api.scriptChanged.label')"
      testid="kube-approval-script-changed"
    >
      <ApprovalValues :values="p.changed" data />
    </ApprovalDetailItem>
  </ApprovalDetails>
</template>
