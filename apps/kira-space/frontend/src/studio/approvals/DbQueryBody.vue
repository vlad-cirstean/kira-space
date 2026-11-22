<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import { computed } from 'vue';
import DbRequestView from './DbRequestView.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import KeptVariablesPanel from './KeptVariablesPanel.vue';

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
      <DbRequestView
        :kind="item.target.kind"
        :statement="payload.statement"
        :kept="payload.kept"
        :params="payload.params"
        :placeholders="payload.placeholders"
      />
    </ApprovalBox>
    <ApprovalBox v-if="payload.script" testid="db-approval-box-transform">
      <JsScriptBlock
        :label="t('studio.dbMcp.approval.query.scriptResult.label')"
        :source="payload.script.source"
        testid="db-approval-script"
      />
    </ApprovalBox>
  </div>
</template>
