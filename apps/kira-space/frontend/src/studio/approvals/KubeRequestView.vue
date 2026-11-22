<script setup lang="ts">
import type { KubeReadPayload } from '@shared/domain/dbmcp';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import { computed } from 'vue';
import KubeRequestLine from './KubeRequestLine.vue';
import PreparedList from './PreparedList.vue';
import { kubePrepared } from './preparedValues';

// The read as the agent sent it: the prepare script's result list, then the kubectl line built from the final
// args (a discover read keeps its plain statement). Read-only, shared by the prompt and the review's Request section.
const props = defineProps<{
  payload: Pick<KubeReadPayload, 'operation' | 'request' | 'changed' | 'statement'> &
    Partial<Pick<KubeReadPayload, 'script'>>;
}>();

const prepared = computed(() => kubePrepared(props.payload));
</script>

<template>
  <PreparedList v-if="prepared.length" :rows="prepared" testid="kube-approval-prepared" />
  <KubeRequestLine v-if="payload.request" :payload="payload" />
  <ApprovalCode v-else-if="payload.statement" data-testid="db-mcp-approval-kube">{{
    payload.statement
  }}</ApprovalCode>
</template>
