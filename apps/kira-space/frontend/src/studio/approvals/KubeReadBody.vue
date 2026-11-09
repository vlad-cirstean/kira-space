<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { kubeReadPayloadSchema } from '@shared/domain/dbmcp';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => kubeReadPayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalCode v-if="payload.statement" data-testid="db-mcp-approval-kube">{{
    payload.statement
  }}</ApprovalCode>
</template>
