<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import { computed } from 'vue';
import ApprovalStatement from './ApprovalStatement.vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => dbQueryPayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalStatement
    :kind="item.target.kind"
    :statement="payload.statement"
    data-testid="db-mcp-approval-body"
  />
</template>
