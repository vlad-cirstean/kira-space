<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { t } from '@theme/i18n';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import { computed } from 'vue';
import { taskCreatePayloadSchema } from './payload';

const props = defineProps<{ item: ApprovalItem }>();
const payload = computed(() => taskCreatePayloadSchema.parse(props.item.payload));
</script>

<template>
  <ApprovalDetails>
    <dt>{{ t('space.common.workflow.label') }}</dt>
    <dd data-testid="ade-task-create-workflow">{{ payload.workflow.name }}</dd>
    <dt>{{ t('space.repo.config.repository.label') }}</dt>
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
        <span class="font-data">{{ r.name }}</span>
        <span class="truncate text-muted-foreground" data-testid="ade-task-create-base">{{
          r.base
            ? t('space.ade.request.taskCreate.base.label', { base: r.base })
            : t('space.ade.request.taskCreate.baseMain.label')
        }}</span>
      </span>
    </dd>
  </ApprovalDetails>
</template>
