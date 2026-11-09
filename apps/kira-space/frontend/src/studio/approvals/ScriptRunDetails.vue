<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { type ScriptValue, scriptRunPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
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
      <dt>{{ t('common.field.description.label') }}</dt>
      <dd class="whitespace-pre-wrap break-words" data-testid="db-mcp-approval-script-description">
        {{ payload.description }}
      </dd>
    </template>
    <template v-if="payload.params.length">
      <dt>{{ t('studio.http.pane.params.label') }}</dt>
      <dd>
        <ApprovalPairs
          :rows="rows(payload.params)"
          testid="db-mcp-approval-script-params"
          :row-testid="(n) => `db-mcp-approval-script-param-${n}`"
        />
      </dd>
    </template>
    <template v-if="payload.env.length">
      <dt>{{ t('studio.dbMcp.approval.detail.env.label') }}</dt>
      <dd>
        <ApprovalPairs
          :rows="rows(payload.env)"
          testid="db-mcp-approval-script-env"
          :row-testid="(n) => `db-mcp-approval-script-env-${n}`"
        />
      </dd>
    </template>
  </ApprovalDetails>
</template>
