<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { kubeReadPayloadSchema } from '@shared/domain/dbmcp';
import { type MessageKey, t } from '@theme/i18n';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => kubeReadPayloadSchema.parse(props.item.payload));

const BODY_KEYS = {
  discover: 'studio.dbMcp.approval.kube.discover.body',
  list: 'studio.dbMcp.approval.kube.list.body',
  describe: 'studio.dbMcp.approval.kube.describe.body',
  logs: 'studio.dbMcp.approval.kube.logs.body',
} as const satisfies Record<string, MessageKey>;
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="db-mcp-approval-kube">
    <p class="m-0 whitespace-pre-wrap">
      {{ t(BODY_KEYS[payload.operation], { cluster: payload.context }) }}
    </p>
    <span class="font-data text-kira-lg" data-testid="db-mcp-approval-kube-namespace">
      {{ payload.namespace || t('kubernetes.namespace.all.option') }}
    </span>
    <span v-if="payload.statement" class="font-data text-kira-sm text-muted-foreground">
      {{ payload.statement }}
    </span>
  </div>
</template>
