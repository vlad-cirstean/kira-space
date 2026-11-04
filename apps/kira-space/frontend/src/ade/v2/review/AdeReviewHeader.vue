<script setup lang="ts">
import { shortSha } from '@kira/git-core';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import Identifier from '@workbench/components/Identifier.vue';
import TitleBarBase from '@workbench/components/TitleBar.vue';
import { computed } from 'vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import { taskTitle } from '../board/labels';
import { taskColor } from '../palette';
import { useAdeReviewWindowStore } from '../state/adeReviewWindow';
import type { ReviewWindowTarget } from '../wire';
import AdeReviewActions from './AdeReviewActions.vue';
import { useReviewContext } from './useReviewContext';
import { useReviewHead } from './useReviewHead';
import { useReviewWorking } from './useReviewWorking';

const props = defineProps<{ target: ReviewWindowTarget }>();
const store = useAdeReviewWindowStore();

const { task, branch, repoLabel } = useReviewContext(() => props.target);
const working = useReviewWorking(() => props.target.branchId);
const title = computed(() =>
  task.value ? taskTitle(task.value, branch.value, '') : props.target.branch,
);
const dirtyCount = computed(() => working.data.value?.files.length ?? 0);
const hint = computed(() =>
  t('space.ade.review.header.uncommitted.hint', { count: dirtyCount.value }),
);
const { head } = useReviewHead(() => props.target);
const headHint = computed(() => {
  const h = head.value;
  if (!h) return '';
  return h.state === 'stale'
    ? t('space.ade.review.actions.headDiffers.hint', { short: h.short })
    : t('space.ade.review.header.headCurrent.hint', { short: h.short });
});
</script>

<template>
  <TitleBarBase>
    <div class="flex min-w-0 flex-1 items-center gap-2" data-testid="ade-review-header">
      <span
        class="inline-block size-2.5 shrink-0 rounded-kira-sm"
        :style="{ background: taskColor(task?.color ?? 0) }"
        data-testid="ade-review-task-colour"
      />
      <span class="min-w-0 truncate text-kira-md font-medium" data-testid="ade-review-title">{{
        title
      }}</span>
      <AdeRepoTag :code-repo-id="target.codeRepoId" :label="repoLabel" />
      <span class="min-w-0 truncate font-data text-kira-sm" data-testid="ade-review-branch">{{
        target.branch
      }}</span>
      <span class="shrink-0 text-kira-sm text-muted-foreground" data-testid="ade-review-base">{{
        t('space.ade.review.header.base.label', { base: target.base })
      }}</span>
      <Tooltip v-if="dirtyCount > 0">
        <TooltipTrigger as-child>
          <Badge
            variant="warn"
            tabindex="0"
            class="wails-no-drag shrink-0"
            :aria-label="hint"
            data-testid="ade-review-uncommitted"
          >
            {{ t('space.ade.review.header.uncommitted.label', { count: dirtyCount }) }}
          </Badge>
        </TooltipTrigger>
        <TooltipContent>{{ hint }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="head">
        <TooltipTrigger as-child>
          <Badge
            :variant="head.state === 'stale' ? 'warn' : 'default'"
            tabindex="0"
            class="wails-no-drag shrink-0"
            :aria-label="headHint"
            :data-head="head.state"
            data-testid="ade-review-head"
          >
            <template v-if="head.state === 'stale'">{{ t('common.stale.status') }}</template>
            <template v-else>
              <CodiconIcon name="check" :size="12" />
              {{ t('space.ade.review.header.headCurrent.label', { short: head.short }) }}
            </template>
          </Badge>
        </TooltipTrigger>
        <TooltipContent>
          <div class="flex flex-col gap-1">
            <span>{{ headHint }}</span>
            <span v-if="head.state === 'stale'" class="flex items-center gap-1.5 text-kira-sm">
              <Identifier :value="head.local" :short="shortSha" />
              <CodiconIcon name="arrow-right" :size="12" />
              <Identifier :value="head.host" :short="shortSha" />
            </span>
            <Identifier v-else :value="head.host" :short="shortSha" />
          </div>
        </TooltipContent>
      </Tooltip>
    </div>
    <div class="wails-no-drag flex shrink-0 items-center gap-1">
      <AdeReviewActions :target="target" />
      <TooltipIconButton
        :icon="store.agentPanelVisible ? 'layout-sidebar-right' : 'layout-sidebar-right-off'"
        :label="t('space.ade.review.agentPanel.toggle.label')"
        :icon-size="16"
        variant="title"
        size="title"
        :aria-pressed="store.agentPanelVisible"
        data-testid="ade-review-toggle-agent"
        @click="store.toggleAgentPanel()"
      />
    </div>
  </TitleBarBase>
</template>
