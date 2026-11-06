<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import ApprovalStatement from './ApprovalStatement.vue';
import PendingResponse from './PendingResponse.vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => dbResponsePayloadSchema.parse(props.item.payload));
// The item id is "db.response:<requestId>"; the held result is fetched by request id.
const requestId = computed(() => props.item.id.slice(props.item.kind.length + 1));
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="db-mcp-preview-body">
    <p class="m-0 whitespace-pre-wrap">
      {{
        t(
          payload.requested
            ? 'studio.dbMcp.preview.requested.body'
            : 'studio.dbMcp.preview.lede.body',
          {
            connection: item.target.name,
            kind: item.target.kind,
          },
        )
      }}
    </p>
    <ApprovalStatement compact :kind="item.target.kind" :statement="payload.statement" />
    <Alert v-if="payload.alreadyRan" variant="warn" data-testid="db-mcp-preview-ran">
      <AlertDescription>{{ t('studio.dbMcp.preview.ran.body') }}</AlertDescription>
    </Alert>
    <PendingResponse :request-id="requestId" :connection-id="payload.connectionId" />
  </div>
</template>
