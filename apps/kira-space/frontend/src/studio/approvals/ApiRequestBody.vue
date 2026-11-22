<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import { computed } from 'vue';
import ApiRequestView from './ApiRequestView.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import KeptVariablesPanel from './KeptVariablesPanel.vue';
import { apiPrepared } from './preparedValues';
import UserInputsPanel from './UserInputsPanel.vue';

// What the agent would send, one box per section: temporary variables, the request (line, body, headers
// and variables), values to type, prepare script, transform script. A credential's value never
// reaches this view. Every place the request reads a temporary variable is marked.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiRequestPayloadSchema.parse(props.item.payload));
// Every use marks its places in the request; a pending one (a value still to type) has no row to show.
const kept = computed(() => payload.value.kept);
const keptRows = computed(() => kept.value.filter((u) => !u.pending));

const prepared = computed(() => apiPrepared(payload.value));
const changed = computed(() => payload.value.script?.prepare?.changed ?? []);
const pairName = (field: 'header' | 'query' | 'form', index: number): string => {
  if (field === 'header') return payload.value.headers[index]?.name ?? '';
  if (field === 'query') return payload.value.query[index]?.name ?? '';
  return payload.value.body?.form?.[index]?.name ?? '';
};
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="api-approval-body">
    <ApprovalBox v-if="keptRows.length" testid="api-approval-box-kept">
      <KeptVariablesPanel :uses="keptRows" :pair-name="pairName" />
    </ApprovalBox>
    <ApprovalBox testid="api-approval-box-request">
      <ApiRequestView
        :method="payload.method"
        :url="payload.url"
        :headers="payload.headers"
        :body="payload.body"
        :variables="payload.variables"
        :kept="kept"
        :changed="changed"
        :prepared="prepared"
        :writes="payload.script?.prepare?.writes ?? []"
      />
    </ApprovalBox>
    <ApprovalBox v-if="payload.inputs.length" testid="api-approval-box-inputs">
      <UserInputsPanel :item-id="item.id" :inputs="payload.inputs" />
    </ApprovalBox>
    <ApprovalBox v-if="payload.script?.prepare" testid="api-approval-box-prepare">
      <JsScriptBlock
        :label="t('studio.dbMcp.approval.api.scriptPrepare.label')"
        :source="payload.script.prepare.source"
        testid="api-approval-script-prepare"
      />
    </ApprovalBox>
    <ApprovalBox v-if="payload.script?.transform" testid="api-approval-box-transform">
      <JsScriptBlock
        :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
        :source="payload.script.transform.source"
        testid="api-approval-script-transform"
      />
    </ApprovalBox>
  </div>
</template>
