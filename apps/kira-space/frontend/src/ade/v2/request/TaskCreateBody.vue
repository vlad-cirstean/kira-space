<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeNotesEditor from '../notes/AdeNotesEditor.vue';
import { taskCreatePayloadSchema } from './payload';

// One orchestrator's request for a child task: what it is called, which workflow and repositories it
// gets, and the brief it starts from. Approve creates it; nothing here edits.
const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => taskCreatePayloadSchema.parse(props.item.payload));
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="ade-task-create-body">
    <KeyValueList>
      <dt>{{ t('space.ade.orchestrator.badge.label') }}</dt>
      <dd class="truncate" data-testid="ade-task-create-parent">{{ payload.parentTitle }}</dd>
      <dt>{{ t('common.field.title.label') }}</dt>
      <dd class="min-w-0 break-words font-medium" data-testid="ade-task-create-title">{{
        payload.title
      }}</dd>
      <dt>{{ t('space.common.workflow.label') }}</dt>
      <dd data-testid="ade-task-create-workflow">{{ payload.workflow.name }}</dd>
      <dt>{{ t('common.module.repositories.label') }}</dt>
      <dd class="flex flex-col gap-0.5" data-testid="ade-task-create-repos">
        <span v-if="!payload.repos.length" class="text-muted-foreground">{{
          t('space.ade.request.taskCreate.noRepos.label')
        }}</span>
        <span
          v-for="r in payload.repos"
          :key="r.codeRepoId"
          class="flex min-w-0 items-baseline gap-2"
          data-testid="ade-task-create-repo"
          :data-code-repo-id="r.codeRepoId"
        >
          <span class="font-medium">{{ r.name }}</span>
          <span class="truncate text-muted-foreground" data-testid="ade-task-create-base">{{
            r.base
              ? t('space.ade.request.taskCreate.base.label', { base: r.base })
              : t('space.ade.request.taskCreate.baseMain.label')
          }}</span>
        </span>
      </dd>
    </KeyValueList>
    <div class="flex flex-col gap-1">
      <span class="text-muted-foreground">{{ t('space.ade.request.taskCreate.brief.label') }}</span>
      <AdeNotesEditor
        :model-value="payload.brief"
        readonly
        compact
        hard-breaks
        :label="t('space.ade.request.taskCreate.brief.label')"
        data-testid="ade-task-create-brief"
      />
    </div>
  </div>
</template>
