<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Skeleton } from '@theme/components/ui/skeleton';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed } from 'vue';
import { useOpenHostView } from '../../../hostview/queries';
import AdeChip from '../AdeChip.vue';
import { gapText } from '../artifacts/gapText';
import type { Tone } from '../board/actions';
import { useTaskIssue } from '../queries';
import { useMarkTrackedSeen } from '../updates/queries';
import { useTaskUpdates } from '../updates/useTaskUpdates';
import type { ArtifactGap } from '../wire';
import { rowMeta } from './adeRow';

// The linked issue as its tracker reports it, inline in the Jira link row: title, status, Jira's PR count and a view
// button.
// The tracker CLI owns the credentials; a failure reads as a line of text, never an error toast.
const props = defineProps<{ taskId: string; link: string }>();
const query = useTaskIssue(
  () => props.taskId,
  () => props.link,
);

const issue = computed(() => query.data.value);
// A task no tracker recognises shows nothing; nothing shifts when the lookup settles.
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

const { pending } = useTaskUpdates();
const markSeen = useMarkTrackedSeen();

function viewIssue(): void {
  if (pending(props.taskId, 'issue'))
    markSeen.mutate({ taskId: props.taskId, itemKeys: ['issue'] });
  // link is `<key> <url>`; the view takes the link when there is one, else the key.
  const [key = '', url = ''] = props.link.trim().split(/\s+/);
  fireAndForget(openView.mutateAsync(url || key), 'ade.taskIssue');
}
</script>

<template>
  <span
    v-if="shown"
    class="flex min-w-0 flex-1 flex-wrap items-center gap-x-1.5"
    data-testid="ade-task-issue"
  >
    <Skeleton v-if="query.isPending.value" class="h-3 w-32" data-testid="ade-task-issue-loading" />
    <template v-else-if="issue">
      <span
        v-if="issue.title"
        class="min-w-0 flex-1 truncate text-muted-foreground"
        data-testid="ade-task-issue-title"
        >{{
          issue.title
        }}</span
      >
      <AdeChip
        v-if="issue.title && issue.status"
        :label="issue.status"
        :tone="CATEGORY_TONE[issue.statusCategory] ?? 'grey'"
        data-testid="ade-task-issue-status"
      />
      <span
        v-if="!issue.reason && issue.summary.known"
        :class="cn(rowMeta, 'shrink-0 whitespace-nowrap')"
        data-testid="ade-task-issue-prs"
      >
        {{
          issue.summary.count > 0
            ? t('space.ade.taskTab.issueLine.prs.label', { count: issue.summary.count })
            : t('space.ade.taskTab.issueLine.prsNone.label')
        }}
      </span>
      <TooltipIconButton
        v-if="issue.title && !issue.reason"
        icon="eye"
        :label="t('space.hostView.issue.view.tooltip')"
        class="shrink-0"
        data-testid="ade-task-issue-view"
        @click="viewIssue"
      />
      <span
        v-if="gap"
        class="basis-full text-kira-sm text-tone-amber"
        data-testid="ade-task-issue-gap"
        >{{
          gapText(gap)
        }}</span
      >
    </template>
  </span>
</template>
