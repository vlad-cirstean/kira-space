<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { AdeNotesEditor } from '../notes/asyncEditor';
import { taskCreatePayloadSchema } from './payload';

// One orchestrator's request for a child task: the brief it starts from. Title, workflow and
// repositories sit on line 1 and in the details. Approve creates it; nothing here edits.
const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => taskCreatePayloadSchema.parse(props.item.payload));
</script>

<template>
  <div data-testid="ade-task-create-body">
    <AdeNotesEditor
      :model-value="payload.brief"
      readonly
      compact
      hard-breaks
      flush
      :label="t('space.ade.request.taskCreate.brief.label')"
      data-testid="ade-task-create-brief"
    />
  </div>
</template>
