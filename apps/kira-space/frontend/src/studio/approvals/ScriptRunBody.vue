<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { scriptRunPayloadSchema } from '@shared/domain/dbmcp';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import { computed } from 'vue';

// What the run would execute: the command. Folder, params and env sit on line 1 and in the details.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => scriptRunPayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalCode data-testid="db-mcp-approval-script-command">{{ payload.command }}</ApprovalCode>
</template>
