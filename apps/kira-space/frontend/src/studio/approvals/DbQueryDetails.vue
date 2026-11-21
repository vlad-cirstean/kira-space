<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import { type MessageKey, t } from '@theme/i18n';
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
    <dt>{{ t('studio.dbMcp.approval.detail.declared.label') }}</dt>
    <dd data-testid="db-approval-detail-mode">{{ p.mode }}</dd>
    <template v-if="p.detected && p.detected !== p.mode">
      <dt>{{ t('studio.dbMcp.approval.detail.looksLike.label') }}</dt>
      <dd>{{ p.detected }}</dd>
    </template>
    <template v-if="p.enforcement && ENFORCEMENT[p.enforcement]">
      <dt>{{ t('common.state.readOnly.status') }}</dt>
      <dd data-testid="db-approval-detail-enforcement">{{ t(ENFORCEMENT[p.enforcement]!) }}</dd>
    </template>
    <template v-if="item.target.readOnly">
      <dt>{{ t('common.field.connection.label') }}</dt>
      <dd>{{ t('studio.dbMcp.approval.detail.connectionReadOnly.body') }}</dd>
    </template>
    <template v-if="p.params.length">
      <dt>{{ t('common.field.parameters.label') }}</dt>
      <dd>{{
        t('studio.dbMcp.approval.detail.paramsBound.status', { count: p.params.length })
      }}</dd>
    </template>
  </ApprovalDetails>
</template>
