<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useElementSize } from '@vueuse/core';
import { useCopy } from '@workbench/behaviour/clipboard';
import { formatRelative } from '@workbench/behaviour/format';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, useTemplateRef } from 'vue';
import { gapText } from '../ade/v2/artifacts/gapText';
import { control } from '../bridge/control';
import CommentComposer from './CommentComposer.vue';
import HostViewFrame from './HostViewFrame.vue';
import IssueComments from './IssueComments.vue';
import IssueSidebar from './IssueSidebar.vue';
import MarkdownBody from './MarkdownBody.vue';
import { issueKey, useIssueDetail } from './queries';
import type { IssueTarget } from './wire';

// One issue: description and comments (left), fields and pull requests (right), stacked when narrow.
// Refresh is the only refetch trigger besides a stale saved copy; a tracker failure shows its reason
// with Retry.
const WIDE_PX = 900;
const props = defineProps<{ target: IssueTarget }>();
const { copy } = useCopy();
const refText = computed(() => props.target.url || props.target.key);
const { query, refresh } = useIssueDetail(refText);
const queryKey = computed(() => issueKey(refText.value));
const result = computed(() => query.data.value);
const compose = computed(() => result.value?.compose);
const detail = computed(() => (result.value?.kind === 'ok' ? result.value.detail : undefined));
const failure = computed(() => {
  if (query.isError.value) return String(query.error.value ?? '');
  const r = result.value;
  if (!r || r.kind === 'ok') return '';
  return gapText({ source: props.target.provider, repo: '', reason: r.reason ?? 'failed' });
});
const title = computed(() => detail.value?.title ?? props.target.key);
const url = computed(() => detail.value?.ref.url || props.target.url);
const label = computed(() => props.target.tracker.label);
const statusVariant = computed(() => {
  switch (detail.value?.statusCategory) {
    case 'done':
      return 'ok' as const;
    case 'indeterminate':
      return 'info' as const;
    default:
      return 'chip' as const;
  }
});
const body = useTemplateRef('body');
const { width } = useElementSize(body);
const stacked = computed(() => width.value < WIDE_PX);

function openOnTracker(): void {
  if (url.value) fireAndForget(control.openExternal(url.value), 'hostView.issue');
}
function copyLink(): void {
  if (url.value) void copy(url.value);
}
</script>

<template>
  <HostViewFrame
    :title="title"
    :ref-text="detail?.type ? `${target.key} ${detail.type}` : target.key"
    :fetching="query.isFetching.value"
    :pending="query.isPending.value"
    :failure="failure"
    :copy-label="t('space.ade.link.copy.tooltip', { label })"
    :open-label="t('space.ade.board.taskMenu.openHost.label', { label })"
    :open-disabled="!url"
    @refresh="refresh"
    @copy="copyLink"
    @open="openOnTracker"
  >
    <template #badge>
      <Badge v-if="detail" :variant="statusVariant" class="shrink-0" data-testid="host-view-state">
        {{ detail.status }}
      </Badge>
    </template>
    <div
      v-if="detail"
      ref="body"
      class="flex min-h-0 flex-1"
      :class="stacked ? 'flex-col overflow-y-auto' : 'flex-row'"
      data-testid="issue-view"
    >
      <main class="min-w-0 flex-1 p-4" :class="stacked ? '' : 'overflow-y-auto'">
        <div
          class="mb-3 flex flex-wrap items-center gap-x-2 text-kira-sm text-muted-foreground"
          data-testid="issue-meta"
        >
          <CodiconIcon name="issues" :size="13" />
          <span>{{
            detail.reporter
              ? t('space.hostView.issue.header.reported.label', {
                  author: detail.reporter.name,
                  when: formatRelative(detail.createdAt),
                })
              : t('space.hostView.issue.header.created.label', {
                  when: formatRelative(detail.createdAt),
                })
          }}</span>
        </div>
        <FieldDescription v-if="detail.stale" class="mb-3" data-testid="issue-stale">{{
          t('space.hostView.issue.stale.hint', { when: formatRelative(detail.fetchedAt) })
        }}</FieldDescription>
        <MarkdownBody :source="detail.description" />
        <IssueComments
          :comments="detail.comments"
          :truncated="!!detail.commentsTruncated"
          :omitted="!!detail.commentsOmitted"
          :label="label"
        />
        <section v-if="compose" class="mt-4 flex flex-col gap-2" data-testid="issue-compose">
          <SectionHeading :label="t('space.hostView.compose.heading.title')" />
          <CommentComposer
            kind="issue"
            :compose="compose"
            :query-key="queryKey"
            :provider="target.provider"
          />
        </section>
      </main>
      <IssueSidebar :detail="detail" :stacked="stacked" />
    </div>
  </HostViewFrame>
</template>
