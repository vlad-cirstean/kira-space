<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Badge } from '@theme/components/ui/badge';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../bridge/control';
import type { ChangeRequestDetail, Check, CheckState, ReviewerState } from './wire';

defineProps<{ detail: ChangeRequestDetail }>();

const REVIEWER_ICON: Record<ReviewerState, string> = {
  approved: 'check',
  changesRequested: 'request-changes',
  rejected: 'error',
  commented: 'comment',
  pending: 'clock',
};
const CHECK_ICON: Record<CheckState, string> = {
  success: 'check',
  failure: 'error',
  error: 'error',
  pending: 'clock',
  skipped: 'debug-step-over',
  neutral: 'circle-outline',
};

function reviewerLabel(state: ReviewerState): string {
  switch (state) {
    case 'approved':
      return t('space.hostView.review.approved.status');
    case 'changesRequested':
      return t('space.hostView.review.changesRequested.status');
    case 'rejected':
      return t('space.hostView.review.rejected.status');
    case 'commented':
      return t('space.hostView.review.commented.status');
    default:
      return t('space.hostView.review.pending.status');
  }
}
function checkLabel(state: CheckState): string {
  switch (state) {
    case 'success':
      return t('space.hostView.check.success.status');
    case 'failure':
      return t('common.state.failed.status');
    case 'error':
      return t('space.hostView.check.error.status');
    case 'pending':
      return t('space.common.pending.status');
    case 'skipped':
      return t('common.skipped.status');
    default:
      return t('space.hostView.check.neutral.status');
  }
}
function openCheck(c: Check): void {
  if (c.url) fireAndForget(control.openExternal(c.url), 'hostView.check');
}
</script>

<template>
  <aside
    class="flex w-72 shrink-0 flex-col gap-5 overflow-y-auto p-4"
    data-testid="host-view-sidebar"
  >
    <section
      v-if="detail.stats || detail.mergeable"
      class="flex flex-col gap-1"
      data-testid="host-view-stats"
    >
      <p v-if="detail.stats" class="m-0 text-kira-sm">{{
        t('space.hostView.stats.summary.label', {
          commits: detail.stats.commits,
          files: detail.stats.files,
          additions: detail.stats.additions,
          deletions: detail.stats.deletions,
        })
      }}</p>
      <p
        v-if="detail.mergeable"
        class="m-0 text-kira-sm"
        :data-mergeable="detail.mergeable"
        data-testid="host-view-mergeable"
        >{{
          detail.mergeable === 'conflicting'
            ? t('space.hostView.mergeable.conflicting.status')
            : t('space.hostView.mergeable.clean.status')
        }}</p
      >
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-reviewers">
      <SectionHeading :label="t('space.hostView.reviewers.heading.title')" />
      <FieldDescription v-if="detail.reviewers.length === 0">{{
        t('space.hostView.reviewers.empty.empty')
      }}</FieldDescription>
      <div
        v-for="r in detail.reviewers"
        :key="r.person.login"
        class="flex items-center gap-2 text-kira-sm"
        :data-state="r.state"
        data-testid="host-view-reviewer"
      >
        <CodiconIcon :name="REVIEWER_ICON[r.state]" :size="13" />
        <span class="min-w-0 flex-1 truncate">{{ r.person.name || r.person.login }}</span>
        <Badge v-if="r.required" variant="chip">{{ t('common.required.label') }}</Badge>
        <span class="shrink-0 text-muted-foreground">{{ reviewerLabel(r.state) }}</span>
      </div>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-checks">
      <SectionHeading :label="t('space.hostView.checks.heading.title')" />
      <FieldDescription v-if="detail.checks.length === 0">{{
        t('space.hostView.checks.empty.empty')
      }}</FieldDescription>
      <div
        v-for="c in detail.checks"
        :key="c.name"
        class="flex items-center gap-2 text-kira-sm"
        :data-state="c.state"
        data-testid="host-view-check"
      >
        <CodiconIcon :name="CHECK_ICON[c.state]" :size="13" />
        <button
          v-if="c.url"
          type="button"
          class="min-w-0 flex-1 truncate border-0 bg-transparent p-0 text-left text-fg hover:underline"
          @click="openCheck(c)"
          >{{
            c.name
          }}</button
        >
        <span v-else class="min-w-0 flex-1 truncate">{{ c.name }}</span>
        <Badge v-if="c.required" variant="chip">{{ t('common.required.label') }}</Badge>
        <span class="shrink-0 text-muted-foreground">{{ checkLabel(c.state) }}</span>
      </div>
      <FieldDescription v-if="detail.checksTruncated" data-testid="host-view-checks-truncated">{{
        t('space.hostView.checks.truncated.hint')
      }}</FieldDescription>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-labels">
      <SectionHeading :label="t('space.hostView.labels.heading.title')" />
      <FieldDescription v-if="detail.labels.length === 0">{{
        t('space.hostView.labels.empty.empty')
      }}</FieldDescription>
      <div class="flex flex-wrap gap-1">
        <Badge
          v-for="l in detail.labels"
          :key="l.name"
          variant="chip"
          data-testid="host-view-label"
        >
          <StatusDot v-if="l.color" :color="`#${l.color}`" />
          {{ l.name }}
        </Badge>
      </div>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-assignees">
      <SectionHeading :label="t('space.hostView.assignees.heading.title')" />
      <FieldDescription v-if="detail.assignees.length === 0">{{
        t('space.hostView.assignees.empty.empty')
      }}</FieldDescription>
      <span
        v-for="a in detail.assignees"
        :key="a.login"
        class="text-kira-sm"
        data-testid="host-view-assignee"
        >{{
          a.name || a.login
        }}</span
      >
    </section>
  </aside>
</template>
