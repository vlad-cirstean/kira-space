<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Skeleton } from '@theme/components/ui/skeleton';
import { t } from '@theme/i18n';
import { issueTone, TONE_BADGE } from '@theme/lib/statusTone';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';
import { gapText } from '../artifacts/gapText';
import { useTaskIssue } from '../queries';
import type { ArtifactGap } from '../wire';
import { rowMeta } from './adeRow';

// The linked issue as its tracker reports it, inline in the Jira link card: title, status and Jira's PR count.
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

const gap = computed<ArtifactGap | null>(() => {
  const i = issue.value;
  if (!i || i.reason === 'noTracker') return null;
  const source = i.provider;
  return i.reason ? { source, repo: '', reason: i.reason } : null;
});
</script>

<template>
  <span
    v-if="shown"
    class="flex min-w-0 flex-1 flex-wrap items-center gap-x-1.5"
    data-testid="ade-task-issue"
  >
    <Skeleton v-if="query.isPending.value" class="h-4 w-1/3" data-testid="ade-task-issue-loading" />
    <template v-else-if="issue">
      <span
        v-if="issue.title"
        class="min-w-0 flex-1 truncate text-muted-foreground"
        data-testid="ade-task-issue-title"
        >{{
          issue.title
        }}</span
      >
      <Badge
        v-if="issue.title && issue.status"
        :variant="TONE_BADGE[issueTone(issue.statusCategory)]"
        data-testid="ade-task-issue-status"
        >{{
          issue.status
        }}</Badge
      >
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
      <span v-if="gap" class="basis-full text-kira-sm text-warn" data-testid="ade-task-issue-gap">{{
        gapText(gap)
      }}</span>
    </template>
  </span>
</template>
