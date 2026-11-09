<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import { computed } from 'vue';
import PendingResponse from './PendingResponse.vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => dbResponsePayloadSchema.parse(props.item.payload));
// The item id is "db.response:<requestId>"; the held result is fetched by request id.
const requestId = computed(() => props.item.id.slice(props.item.kind.length + 1));
</script>

<template>
  <div data-testid="db-mcp-preview-body">
    <PendingResponse :request-id="requestId" :connection-id="payload.connectionId" />
  </div>
</template>
