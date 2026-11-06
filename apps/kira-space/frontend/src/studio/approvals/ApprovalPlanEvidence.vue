<script setup lang="ts">
import type { DbMcpApprovalPlan } from '@shared/domain/dbmcp';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { formatCount } from '@workbench/behaviour/format';

defineProps<{ plan: DbMcpApprovalPlan }>();
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="db-mcp-approval-plan">
    <FieldDescription data-testid="db-mcp-approval-plan-rows">
      <template v-if="plan.estimatedRowsRead !== null">
        {{
          t('studio.dbMcp.approval.estimate.body', {
            count: plan.estimatedRowsRead,
            threshold: formatCount(plan.thresholdRows),
          })
        }}
      </template>
      <template v-else>{{ t('studio.console.plan.noEstimate.body') }}</template>
    </FieldDescription>
    <FieldDescription
      v-for="(issue, i) in plan.issues"
      :key="i"
      data-testid="db-mcp-approval-plan-issue"
    >
      <strong>{{ issue.severity }}</strong> {{ issue.message }}
    </FieldDescription>
    <FieldDescription v-if="plan.issuesOmitted > 0">
      {{ t('common.more.label', { count: plan.issuesOmitted }) }}
    </FieldDescription>
  </div>
</template>
