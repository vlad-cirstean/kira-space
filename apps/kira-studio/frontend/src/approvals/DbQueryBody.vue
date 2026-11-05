<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import { FieldDescription } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import { formatCount } from '@workbench/behaviour/format';
import { computed } from 'vue';
import ApprovalStatement from './ApprovalStatement.vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => dbQueryPayloadSchema.parse(props.item.payload));

const CLASS_BODY_KEYS: Record<string, MessageKey> = {
  read: 'studio.dbMcp.approval.read.body',
  write: 'studio.dbMcp.approval.write.body',
  ddl: 'studio.dbMcp.approval.ddl.body',
  unknown: 'studio.dbMcp.approval.unknown.body',
};

const lede = computed(() => {
  const { plan } = payload.value;
  const params = { connection: props.item.target.name, kind: props.item.target.kind };
  if (props.item.reason === 'heavy' && plan && plan.estimatedRowsRead !== null) {
    return t('studio.dbMcp.approval.heavy.body', {
      ...params,
      count: plan.estimatedRowsRead,
      threshold: formatCount(plan.thresholdRows),
    });
  }
  return t(
    CLASS_BODY_KEYS[props.item.operation.class] ?? 'studio.dbMcp.approval.other.body',
    params,
  );
});
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="db-mcp-approval-body">
    <p class="m-0 whitespace-pre-wrap">{{ lede }}</p>
    <ApprovalStatement :kind="item.target.kind" :statement="payload.statement" />
    <div v-if="payload.plan" class="flex flex-col gap-1" data-testid="db-mcp-approval-plan">
      <FieldDescription data-testid="db-mcp-approval-plan-rows">
        <template v-if="payload.plan.estimatedRowsRead !== null">
          {{
            t('studio.dbMcp.approval.estimate.body', {
              count: payload.plan.estimatedRowsRead,
              threshold: formatCount(payload.plan.thresholdRows),
            })
          }}
        </template>
        <template v-else>{{ t('studio.console.plan.noEstimate.body') }}</template>
      </FieldDescription>
      <FieldDescription
        v-for="(issue, i) in payload.plan.issues"
        :key="i"
        data-testid="db-mcp-approval-plan-issue"
      >
        <strong>{{ issue.severity }}</strong> {{ issue.message }}
      </FieldDescription>
      <FieldDescription v-if="payload.plan.issuesOmitted > 0">
        {{ t('common.more.label', { count: payload.plan.issuesOmitted }) }}
      </FieldDescription>
    </div>
  </div>
</template>
