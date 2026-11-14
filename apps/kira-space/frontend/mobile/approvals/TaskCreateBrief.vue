<script setup lang="ts">
import { taskCreatePayloadSchema } from '@ade/request/payload';
import type { ApprovalItem } from '@shared/domain/approvals';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// The brief an orchestrator starts a child from, as plain text. The desktop's notes editor chunk
// imports the Wails bridge, which the phone build lacks; the phone reads the brief, never edits it.
const props = defineProps<{ item: ApprovalItem }>();
const brief = computed(() => taskCreatePayloadSchema.parse(props.item.payload).brief);
</script>

<template>
  <div class="whitespace-pre-wrap text-kira-md text-fg" data-testid="ade-task-create-brief">
    <span class="sr-only">{{ t('space.ade.request.taskCreate.brief.label') }}</span>
    {{ brief }}
  </div>
</template>
