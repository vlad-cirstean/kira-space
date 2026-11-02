<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Skeleton } from '@theme/components/ui/skeleton';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed } from 'vue';
import { useOpenHostView } from '../../../hostview/queries';
import AdeChip from '../AdeChip.vue';
import { gapText } from '../artifacts/gapText';
import type { Tone } from '../board/actions';
import { useTaskIssue } from '../queries';
import type { ArtifactGap } from '../wire';

// The linked issue as its tracker reports it: title, status and Jira's PR count.
// The tracker CLI owns the credentials; a failure reads as a line of text, never an error toast.
const props = defineProps<{ taskId: string; link: string }>();
const query = useTaskIssue(
  () => props.taskId,
  () => props.link,
);

const issue = computed(() => query.data.value);
// A task no tracker recognises shows no row at all; nothing shifts when the lookup settles.
const shown = computed(
  () => query.isPending.value || (issue.value !== undefined && issue.value.reason !== 'noTracker'),
);
const CATEGORY_TONE: Record<string, Tone> = { new: 'grey', indeterminate: 'blue', done: 'green' };

const gap = computed<ArtifactGap | null>(() => {
  const i = issue.value;
  if (!i || i.reason === 'noTracker') return null;
  const source = i.provider;
  return i.reason ? { source, repo: '', reason: i.reason } : null;
});

const openView = useOpenHostView();

function viewIssue(): void {
  // link is `<key> <url>`; the view takes the link when there is one, else the key.
  const [key = '', url = ''] = props.link.trim().split(/\s+/);
  fireAndForget(openView.mutateAsync(url || key), 'ade.taskIssue');
}
</script>

<template>
  <template v-if="shown">
    <dt />
    <dd class="flex min-w-0 flex-col gap-1" data-testid="ade-task-issue">
      <Skeleton
        v-if="query.isPending.value"
        class="h-4 w-48"
        data-testid="ade-task-issue-loading"
      />
      <template v-else-if="issue">
        <div v-if="issue.title" class="flex min-w-0 items-center gap-2">
          <span class="min-w-0 truncate text-kira-md" data-testid="ade-task-issue-title">{{
            issue.title
          }}</span>
          <AdeChip
            v-if="issue.status"
            :label="issue.status"
            :tone="CATEGORY_TONE[issue.statusCategory] ?? 'grey'"
            data-testid="ade-task-issue-status"
          />
          <TooltipIconButton
            v-if="!issue.reason"
            icon="eye"
            :label="t('space.hostView.issue.view.tooltip')"
            :icon-size="16"
            class="shrink-0"
            data-testid="ade-task-issue-view"
            @click="viewIssue"
          />
        </div>
        <template v-if="!issue.reason">
          <div
            v-if="issue.summary.known"
            class="text-kira-sm text-subtle"
            data-testid="ade-task-issue-prs"
          >
            {{
              issue.summary.count > 0
                ? t('space.ade.taskTab.issueLine.prs.label', { count: issue.summary.count })
                : t('space.ade.taskTab.issueLine.prsNone.label')
            }}
          </div>
        </template>
        <div v-if="gap" class="text-kira-sm text-tone-amber" data-testid="ade-task-issue-gap">{{
          gapText(gap)
        }}</div>
      </template>
    </dd>
  </template>
</template>
