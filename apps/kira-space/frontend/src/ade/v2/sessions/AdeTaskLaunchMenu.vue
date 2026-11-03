<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import MenuIconButton from '@workbench/components/MenuIconButton.vue';
import { useTaskLaunch } from './useTaskLaunch';

// The plus on a task's Sessions tab bar: per worktree, a terminal (Git module) or an agent session with no first message.
const props = defineProps<{ taskId: string; branchId?: string }>();

const { usable, pending, items } = useTaskLaunch(
  () => props.taskId,
  () => props.branchId,
);
</script>

<template>
  <MenuIconButton
    v-if="usable"
    :label="t('space.ade.taskLaunch.trigger.tooltip')"
    :items="items"
    :pending="pending"
    class="shrink-0"
    data-testid="ade-task-launch"
  />
  <TooltipIconButton
    v-else
    icon="add"
    :label="t('space.ade.taskLaunch.reason.noWorktree.label')"
    class="shrink-0"
    disabled-trigger
    disabled
    data-testid="ade-task-launch"
  />
</template>
