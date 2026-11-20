<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { paneFrame, windowBody } from '@theme/lib/frames';
import TitleBarBase from '@workbench/components/TitleBar.vue';
import TitleBarTitle from '@workbench/components/TitleBarTitle.vue';
import { computed } from 'vue';
import { taskTitle } from '../board/labels';
import { taskColor } from '../palette';
import { useBoard } from '../queries';
import { useAdeFilesWindowStore } from '../state/adeFilesWindow';
import AdeTaskFiles from './AdeTaskFiles.vue';

// One task's worktree files in their own window: the task panel's Files icon opens it.
const store = useAdeFilesWindowStore();
const board = useBoard();
const taskId = computed(() => store.taskId ?? '');
const task = computed(() => board.data.value?.tasks.find((x) => x.id === taskId.value));
const title = computed(() => {
  const tk = task.value;
  if (!tk) return '';
  const first = board.data.value?.branches.find((b) => b.id === tk.branchIds[0]);
  return taskTitle(tk, first, '');
});
</script>

<template>
  <div class="flex h-full flex-col" data-testid="ade-files-window">
    <TitleBarBase>
      <div class="flex min-w-0 flex-1 items-center gap-2" data-testid="ade-files-window-header">
        <span
          class="inline-block size-2.5 shrink-0 rounded-kira-sm"
          :style="{ background: taskColor(task?.color ?? 0) }"
          data-testid="ade-files-window-colour"
        />
        <TitleBarTitle data-testid="ade-files-window-title">{{ title }}</TitleBarTitle>
        <span class="shrink-0 text-kira-sm text-muted-foreground">{{
          t('common.module.files.label')
        }}</span>
      </div>
    </TitleBarBase>
    <div :class="windowBody">
      <section :class="paneFrame" class="min-w-0 flex-1">
        <EmptyState
          v-if="board.data.value && !task"
          fill
          icon="folder"
          :title="t('space.ade.files.window.gone.empty')"
          data-testid="ade-files-window-gone"
        />
        <AdeTaskFiles v-else-if="task" :task-id="taskId" />
      </section>
    </div>
  </div>
</template>
