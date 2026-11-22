<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import { computed } from 'vue';
import ApprovalStatement from './ApprovalStatement.vue';
import DbParamsList from './DbParamsList.vue';
import KeptVariablesPanel from './KeptVariablesPanel.vue';
import TransformScriptBlock from './TransformScriptBlock.vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => dbQueryPayloadSchema.parse(props.item.payload));
</script>

<template>
  <div class="flex flex-col gap-2">
    <ApprovalBox v-if="payload.kept.length" testid="db-approval-box-kept">
      <KeptVariablesPanel
        :uses="payload.kept"
        :pair-name="() => ''"
        :title="t('studio.kept.panel.query.label')"
        testid-prefix="db-approval"
      />
    </ApprovalBox>
    <ApprovalBox testid="db-approval-box-statement">
      <div class="flex flex-col gap-1">
        <ApprovalStatement
          :kind="item.target.kind"
          :statement="payload.statement"
          :kept="payload.kept"
          data-testid="db-mcp-approval-body"
        />
        <DbParamsList
          v-if="payload.params.length"
          :params="payload.params"
          :placeholders="payload.placeholders"
          :kept="payload.kept"
        />
      </div>
    </ApprovalBox>
    <ApprovalBox v-if="payload.script" testid="db-approval-box-transform">
      <TransformScriptBlock
        :label="t('studio.dbMcp.approval.query.scriptResult.label')"
        :script="payload.script"
        :kept="payload.kept"
        testid="db-approval"
        script-testid="db-approval-script"
      />
    </ApprovalBox>
  </div>
</template>
