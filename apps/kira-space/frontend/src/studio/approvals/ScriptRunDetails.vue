<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { type ScriptValue, scriptRunPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import ApprovalPairs from '@workbench/approvals/ApprovalPairs.vue';
import { computed } from 'vue';

// A secret's value never reaches this view; a saved one is named, not shown.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => scriptRunPayloadSchema.parse(props.item.payload));

function rows(values: ScriptValue[]): { name: string; value: string }[] {
  return values.map((v) => ({
    name: v.name,
    value: v.secret
      ? v.saved
        ? t('studio.dbMcp.approval.script.secretSet.status')
        : t('studio.dbMcp.approval.script.secretMissing.status')
      : v.value,
  }));
}
</script>

<template>
  <ApprovalDetails v-if="payload.description || payload.params.length || payload.env.length">
    <template v-if="payload.description">
      <ApprovalDetailItem
        :label="t('common.field.description.label')"
        testid="db-mcp-approval-script-description"
      >
        <span class="whitespace-pre-wrap break-words">{{ payload.description }}</span>
      </ApprovalDetailItem>
    </template>
    <template v-if="payload.params.length">
      <ApprovalDetailItem :label="t('studio.http.pane.params.label')">
        <ApprovalPairs
          :rows="rows(payload.params)"
          density="approval"
          testid="db-mcp-approval-script-params"
          :row-testid="(n) => `db-mcp-approval-script-param-${n}`"
        />
      </ApprovalDetailItem>
    </template>
    <template v-if="payload.env.length">
      <ApprovalDetailItem :label="t('studio.dbMcp.approval.detail.env.label')">
        <ApprovalPairs
          :rows="rows(payload.env)"
          density="approval"
          testid="db-mcp-approval-script-env"
          :row-testid="(n) => `db-mcp-approval-script-env-${n}`"
        />
      </ApprovalDetailItem>
    </template>
  </ApprovalDetails>
</template>
