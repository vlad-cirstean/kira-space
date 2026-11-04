<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { formatRelative } from '@workbench/behaviour/format';
import { ref } from 'vue';
import CommentComposer from './CommentComposer.vue';
import MarkdownBody from './MarkdownBody.vue';
import type { HostViewCompose, ReviewerState, Thread } from './wire';

// `compose` set means the window can post: each thread of a reply kind offers Reply.
const props = defineProps<{
  threads: Thread[];
  truncated: boolean;
  compose?: HostViewCompose;
  queryKey?: readonly unknown[];
  provider?: string;
}>();

const replying = ref<string | null>(null);
function canReply(thread: Thread): boolean {
  return !!props.compose?.replyKinds.includes(thread.kind) && !thread.id.startsWith('pending-');
}

function reviewLabel(state: ReviewerState | undefined): string {
  switch (state) {
    case 'approved':
      return t('space.hostView.review.approved.status');
    case 'changesRequested':
      return t('space.hostView.review.changesRequested.status');
    case 'rejected':
      return t('space.hostView.review.rejected.status');
    default:
      return t('space.hostView.review.commented.status');
  }
}
</script>

<template>
  <section class="mt-6 flex flex-col gap-3" data-testid="host-view-conversation">
    <SectionHeading :label="t('space.hostView.conversation.heading.title')" />
    <FieldDescription v-if="threads.length === 0" data-testid="host-view-no-threads">{{
      t('space.hostView.conversation.empty.empty')
    }}</FieldDescription>
    <Collapsible
      v-for="thread in threads"
      :key="thread.id"
      as="article"
      :default-open="!thread.resolved"
      class="rounded-kira"
      :data-kind="thread.kind"
      data-testid="host-view-thread"
    >
      <header
        v-if="thread.kind !== 'comment' || thread.resolved"
        class="flex flex-wrap items-center gap-2 px-3 py-1.5 text-kira-sm"
      >
        <Badge
          v-if="thread.kind === 'review'"
          variant="chip"
          data-testid="host-view-thread-review"
          >{{
            reviewLabel(thread.reviewState)
          }}</Badge
        >
        <span v-if="thread.path" class="font-data" data-testid="host-view-thread-path">{{
          thread.line ? `${thread.path}:${thread.line}` : thread.path
        }}</span>
        <Badge v-if="thread.resolved" variant="ok">{{
          t('space.hostView.conversation.resolved.status')
        }}</Badge>
        <Badge v-if="thread.outdated" variant="warn">{{
          t('space.hostView.conversation.outdated.status')
        }}</Badge>
        <CollapsibleTrigger
          v-if="thread.resolved"
          class="ml-auto"
          :aria-label="t('space.hostView.conversation.toggle.aria')"
          data-testid="host-view-thread-toggle"
        >
          <CodiconIcon name="chevron-down" :size="13" />
        </CollapsibleTrigger>
      </header>
      <CollapsibleContent>
        <FieldDescription
          v-if="thread.commentsTruncated"
          class="px-3 py-1.5"
          data-testid="host-view-comments-truncated"
          >{{
            t('space.hostView.conversation.commentsTruncated.hint')
          }}</FieldDescription
        >
        <div
          v-for="comment in thread.comments"
          :key="comment.id"
          class="px-3 py-2"
          :class="comment.pending && 'text-muted-foreground'"
          :data-pending="comment.pending || undefined"
          data-testid="host-view-comment"
        >
          <div class="m-0 mb-1 text-kira-sm text-muted-foreground">
            <span class="font-medium text-fg">{{
              comment.author.name || comment.author.login
            }}</span>
            · {{ formatRelative(comment.createdAt) }}
          </div>
          <MarkdownBody :source="comment.body" />
        </div>
        <div v-if="canReply(thread)" class="px-3 py-2">
          <Button
            v-if="replying !== thread.id"
            variant="toolbar"
            size="kira"
            data-testid="host-view-thread-reply"
            @click="replying = thread.id"
          >
            {{ t('space.common.reply.label') }}
          </Button>
          <CommentComposer
            v-else-if="compose && queryKey"
            compact
            kind="changeRequest"
            :compose="compose"
            :query-key="queryKey"
            :provider="provider ?? ''"
            :reply-to="thread.id"
            @posted="replying = null"
            @cancel="replying = null"
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
    <FieldDescription v-if="truncated" data-testid="host-view-threads-truncated">{{
      t('space.hostView.conversation.truncated.hint')
    }}</FieldDescription>
  </section>
</template>
