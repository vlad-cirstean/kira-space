<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { kubeReadPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import { computed } from 'vue';
import JsScriptBlock from './JsScriptBlock.vue';
import KeptVariablesPanel from './KeptVariablesPanel.vue';
import KubeRequestView from './KubeRequestView.vue';

// What the agent would read, one box per section: the kept variables its scripts read, the kubectl
// line, then the scripts. Discover has no final args, so it keeps the plain statement.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => kubeReadPayloadSchema.parse(props.item.payload));
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="kube-approval-body">
    <ApprovalBox v-if="payload.kept.length" testid="kube-approval-box-kept">
      <KeptVariablesPanel
        :uses="payload.kept"
        :pair-name="() => ''"
        testid-prefix="kube-approval"
      />
    </ApprovalBox>
    <ApprovalBox v-if="payload.request || payload.statement" testid="kube-approval-box-request">
      <KubeRequestView :payload="payload" />
    </ApprovalBox>
    <ApprovalBox v-if="payload.script?.prepare" testid="kube-approval-box-prepare">
      <JsScriptBlock
        :label="t('studio.dbMcp.approval.api.scriptPrepare.label')"
        :source="payload.script.prepare.source"
        testid="kube-approval-script-prepare"
      />
    </ApprovalBox>
    <ApprovalBox v-if="payload.script?.transform" testid="kube-approval-box-transform">
      <JsScriptBlock
        :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
        :source="payload.script.transform.source"
        testid="kube-approval-script-transform"
      />
    </ApprovalBox>
  </div>
</template>
