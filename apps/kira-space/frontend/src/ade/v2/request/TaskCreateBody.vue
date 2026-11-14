<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { computed } from 'vue';
import AdeNoteBody from '../notes/AdeNoteBody.vue';
import { taskCreatePayloadSchema } from './payload';

// One orchestrator's request for a child task: the brief it starts from. Title, workflow and
// repositories sit on line 1 and in the details. Approve creates it; nothing here edits, so the
// brief renders as static HTML (no lazy editor chunk) and paints in the dialog's first frame.
const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => taskCreatePayloadSchema.parse(props.item.payload));
</script>

<template>
  <div data-testid="ade-task-create-body">
    <AdeNoteBody :body="payload.brief" hard-breaks flush data-testid="ade-task-create-brief" />
  </div>
</template>
