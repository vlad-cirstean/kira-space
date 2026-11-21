<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { kubeReadPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import { computed } from 'vue';
import { keptScriptRanges } from '../kept/keptHighlight';
import JsScriptBlock from './JsScriptBlock.vue';
import KeptVariablesPanel from './KeptVariablesPanel.vue';
import KubeRequestLine from './KubeRequestLine.vue';
import TransformScriptBlock from './TransformScriptBlock.vue';

// What the agent would read: the kept variables its scripts read, the kubectl line, then the scripts. Discover
// has no final args, so it keeps the plain statement.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => kubeReadPayloadSchema.parse(props.item.payload));
const keptSites = computed(() => payload.value.kept.flatMap((u) => u.sites));
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="kube-approval-body">
    <KeptVariablesPanel
      v-if="payload.kept.length"
      :uses="payload.kept"
      :pair-name="() => ''"
      testid-prefix="kube-approval"
    />
    <KubeRequestLine v-if="payload.request" :payload="payload" />
    <ApprovalCode v-else-if="payload.statement" data-testid="db-mcp-approval-kube">{{
      payload.statement
    }}</ApprovalCode>
    <JsScriptBlock
      v-if="payload.script?.prepare"
      :label="t('studio.dbMcp.approval.api.scriptPrepare.label')"
      :source="payload.script.prepare.source"
      :range-highlights="payload.kept.length ? keptScriptRanges(keptSites, 'prepare') : undefined"
      testid="kube-approval-script-prepare"
    />
    <TransformScriptBlock
      v-if="payload.script?.transform"
      :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
      :script="payload.script.transform"
      :kept="payload.kept"
      testid="kube-approval"
      script-testid="kube-approval-script-transform"
    />
  </div>
</template>
