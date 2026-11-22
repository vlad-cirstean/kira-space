<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import { Badge } from '@theme/components/ui/badge';
import { type MessageKey, t } from '@theme/i18n';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();
const p = computed(() => dbQueryPayloadSchema.parse(props.item.payload));
const ENFORCEMENT: Record<string, MessageKey> = {
  session: 'studio.dbMcp.approval.enforcement.session.body',
  app: 'studio.dbMcp.approval.enforcement.app.body',
  none: 'studio.dbMcp.approval.enforcement.none.body',
};
</script>

<template>
  <ApprovalDetails>
    <ApprovalDetailItem
      :label="t('studio.dbMcp.approval.detail.declared.label')"
      testid="db-approval-detail-mode"
    >
      <Badge>{{ p.mode }}</Badge>
    </ApprovalDetailItem>
    <ApprovalDetailItem
      v-if="p.detected && p.detected !== p.mode"
      :label="t('studio.dbMcp.approval.detail.looksLike.label')"
    >
      <Badge>{{ p.detected }}</Badge>
    </ApprovalDetailItem>
    <ApprovalDetailItem
      v-if="p.enforcement && ENFORCEMENT[p.enforcement]"
      :label="t('common.state.readOnly.status')"
      testid="db-approval-detail-enforcement"
    >
      {{ t(ENFORCEMENT[p.enforcement]!) }}
    </ApprovalDetailItem>
    <ApprovalDetailItem v-if="item.target.readOnly" :label="t('common.field.connection.label')">
      {{ t('studio.dbMcp.approval.detail.connectionReadOnly.body') }}
    </ApprovalDetailItem>
    <ApprovalDetailItem v-if="p.params.length" :label="t('common.field.parameters.label')">
      {{ t('studio.dbMcp.approval.detail.paramsBound.status', { count: p.params.length }) }}
    </ApprovalDetailItem>
  </ApprovalDetails>
</template>
