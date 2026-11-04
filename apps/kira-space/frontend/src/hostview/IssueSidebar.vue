<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed } from 'vue';
import { initials, statusVariant } from './issueFormat';
import { useOpenHostView } from './queries';
import RelativeTime from './RelativeTime.vue';
import type { IssueDetail } from './wire';

const props = defineProps<{ detail: IssueDetail; stacked: boolean }>();
const open = useOpenHostView();
const tone = computed(() => statusVariant(props.detail.statusCategory));

function openView(urlOrKey: string): void {
  fireAndForget(open.mutateAsync(urlOrKey), 'hostView.issue');
}
</script>

<template>
  <aside
    class="flex shrink-0 flex-col gap-5 overflow-y-auto p-4"
    :class="stacked ? '' : 'w-72'"
    data-testid="issue-sidebar"
  >
    <KeyValueList class="items-baseline gap-y-2">
      <dt>{{ t('common.field.status.label') }}</dt>
      <dd>
        <Badge :variant="tone" data-testid="issue-side-status">{{ detail.status }}</Badge>
      </dd>
      <template v-if="detail.type">
        <dt>{{ t('common.field.type.label') }}</dt>
        <dd data-testid="issue-side-type">{{ detail.type }}</dd>
      </template>
      <template v-if="detail.priority">
        <dt>{{ t('space.hostView.issue.field.priority.label') }}</dt>
        <dd data-testid="issue-side-priority">{{ detail.priority }}</dd>
      </template>
      <template v-if="detail.resolution">
        <dt>{{ t('space.hostView.issue.field.resolution.label') }}</dt>
        <dd data-testid="issue-side-resolution">{{ detail.resolution }}</dd>
      </template>
      <dt>{{ t('space.hostView.issue.field.assignee.label') }}</dt>
      <dd class="flex items-center gap-1.5" data-testid="issue-side-assignee">
        <span
          v-if="detail.assignee"
          class="grid size-5 shrink-0 place-items-center rounded-full bg-field text-kira-sm"
          aria-hidden="true"
          >{{
            initials(detail.assignee.name)
          }}</span
        >{{ detail.assignee?.name || t('space.hostView.issue.assignee.none.empty') }}
      </dd>
      <dt>{{ t('space.hostView.issue.field.reporter.label') }}</dt>
      <dd class="flex items-center gap-1.5" data-testid="issue-side-reporter">
        <span
          v-if="detail.reporter"
          class="grid size-5 shrink-0 place-items-center rounded-full bg-field text-kira-sm"
          aria-hidden="true"
          >{{
            initials(detail.reporter.name)
          }}</span
        >{{ detail.reporter?.name || t('space.hostView.issue.reporter.none.empty') }}
      </dd>
      <template v-if="detail.createdAt">
        <dt>{{ t('common.created.label') }}</dt>
        <dd data-testid="issue-side-created"><RelativeTime :at="detail.createdAt" /></dd>
      </template>
      <template v-if="detail.updatedAt">
        <dt>{{ t('space.hostView.issue.date.updated.label') }}</dt>
        <dd data-testid="issue-side-updated"><RelativeTime :at="detail.updatedAt" /></dd>
      </template>
      <template v-if="detail.dueDate">
        <dt>{{ t('space.hostView.issue.date.due.label') }}</dt>
        <dd data-testid="issue-side-due">{{ detail.dueDate }}</dd>
      </template>
    </KeyValueList>

    <section v-if="detail.parent" class="flex flex-col gap-1.5" data-testid="issue-side-parent">
      <SectionHeading :label="t('space.hostView.issue.parent.heading.title')" />
      <Button
        size="inline"
        variant="link"
        class="justify-start"
        data-testid="issue-side-parent-open"
        @click="openView(detail.parent.key)"
      >
        {{
          detail.parent.title ? `${detail.parent.key} ${detail.parent.title}` : detail.parent.key
        }}
      </Button>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="issue-side-labels">
      <SectionHeading :label="t('space.hostView.labels.heading.title')" />
      <FieldDescription v-if="detail.labels.length === 0">{{
        t('space.hostView.labels.empty.empty')
      }}</FieldDescription>
      <div class="flex flex-wrap gap-1">
        <Badge v-for="l in detail.labels" :key="l" variant="chip" data-testid="issue-side-label">{{
          l
        }}</Badge>
      </div>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="issue-side-components">
      <SectionHeading :label="t('space.hostView.issue.components.heading.title')" />
      <FieldDescription v-if="detail.components.length === 0">{{
        t('space.hostView.issue.components.empty.empty')
      }}</FieldDescription>
      <div class="flex flex-wrap gap-1">
        <Badge
          v-for="c in detail.components"
          :key="c"
          variant="chip"
          data-testid="issue-side-component"
          >{{
            c
          }}</Badge
        >
      </div>
    </section>

    <section
      v-if="detail.summary.known"
      class="flex flex-col gap-1.5"
      data-testid="issue-side-pull-requests"
    >
      <SectionHeading :label="t('space.hostView.issue.pullRequests.heading.title')" />
      <FieldDescription data-testid="issue-side-pull-requests-count">{{
        detail.summary.count > 0
          ? t('space.ade.taskTab.issueLine.prs.label', { count: detail.summary.count })
          : t('space.ade.taskTab.issueLine.prsNone.label')
      }}</FieldDescription>
    </section>
  </aside>
</template>
