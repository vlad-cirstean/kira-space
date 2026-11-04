<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../bridge/control';
import { initials } from './issueFormat';
import MarkdownBody from './MarkdownBody.vue';
import RelativeTime from './RelativeTime.vue';
import type { IssueComment } from './wire';

// Read-only comments, oldest first, with a note when the list is partial.
defineProps<{
  comments: IssueComment[];
  truncated: boolean;
  omitted: boolean;
  label: string;
}>();

function openComment(url: string): void {
  fireAndForget(control.openExternal(url), 'hostView.issue');
}
</script>

<template>
  <section class="flex flex-col gap-3" data-testid="issue-comments">
    <SectionHeading :label="t('space.hostView.conversation.heading.title')" />
    <FieldDescription v-if="comments.length === 0 && !omitted" data-testid="issue-comments-empty">{{
      t('space.hostView.conversation.empty.empty')
    }}</FieldDescription>
    <FieldDescription v-if="omitted" data-testid="issue-comments-omitted">{{
      t('space.hostView.issue.comments.omitted.hint', { label })
    }}</FieldDescription>
    <FieldDescription v-else-if="truncated" data-testid="issue-comments-truncated">{{
      t('space.hostView.issue.comments.truncated.hint', { label })
    }}</FieldDescription>
    <article
      v-for="comment in comments"
      :key="comment.id"
      class="flex flex-col gap-1 pt-3 first:pt-0"
      :class="comment.pending && 'text-muted-foreground'"
      :data-pending="comment.pending || undefined"
      data-testid="issue-comment"
    >
      <div class="flex items-center gap-2 text-kira-sm text-muted-foreground">
        <span
          class="grid size-6 shrink-0 place-items-center rounded-full bg-field text-kira-sm text-fg"
          aria-hidden="true"
          >{{
            initials(comment.author.name)
          }}</span
        >
        <span class="font-medium text-fg">{{ comment.author.name }}</span>
        <RelativeTime :at="comment.createdAt" />
        <span
          v-if="comment.updatedAt && comment.updatedAt > comment.createdAt"
          data-testid="issue-comment-edited"
          >· {{ t('space.hostView.issue.comments.edited.status') }}</span
        >
        <TooltipIconButton
          v-if="comment.url"
          icon="link-external"
          class="ml-auto"
          :label="t('space.hostView.issue.comments.open.tooltip', { label })"
          data-testid="issue-comment-open"
          @click="openComment(comment.url)"
        />
      </div>
      <div class="pl-8">
        <MarkdownBody :source="comment.body" />
      </div>
    </article>
  </section>
</template>
