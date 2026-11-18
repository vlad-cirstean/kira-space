<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { changeRequestTone, TONE_BADGE } from '@theme/lib/statusTone';
import { useCopy } from '@workbench/behaviour/clipboard';
import { formatRelative } from '@workbench/behaviour/format';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed } from 'vue';
import { control } from '../bridge/control';
import CommentComposer from './CommentComposer.vue';
import HostViewConversation from './HostViewConversation.vue';
import HostViewFrame from './HostViewFrame.vue';
import HostViewSidebar from './HostViewSidebar.vue';
import MarkdownBody from './MarkdownBody.vue';
import { changeRequestKey, useChangeRequestDetail } from './queries';
import type { ChangeRequestTarget } from './wire';

// One change request: description and conversation (left), state and people (right). Refresh is the
// only refetch trigger; a host failure shows its reason with Retry.
const props = defineProps<{ target: ChangeRequestTarget }>();
const target = computed(() => props.target);
const { copy } = useCopy();
const query = useChangeRequestDetail(() => target.value.url);
const result = computed(() => query.data.value);
const compose = computed(() => result.value?.compose);
const queryKey = computed(() => changeRequestKey(target.value.url));
const detail = computed(() => (result.value?.kind === 'ok' ? result.value.detail : undefined));
const failure = computed(() => {
  if (query.isError.value) return String(query.error.value ?? '');
  const r = result.value;
  if (!r || r.kind === 'ok') return '';
  return r.status.reason || t('space.hostView.gone.body');
});
const title = computed(
  () => detail.value?.title ?? `${target.value.host.changeRequestShort}${target.value.number}`,
);
const stateLabel = computed(() => {
  switch (detail.value?.state) {
    case 'draft':
      return t('common.draft.status');
    case 'merged':
      return t('common.merged.status');
    case 'closed':
      return t('common.closed.status');
    default:
      return t('common.action.open.label');
  }
});

function refresh(): void {
  void query.refetch();
}
function openOnHost(): void {
  fireAndForget(control.gitHostOpenChangeRequestUrl(target.value.url), 'hostView.window');
}
function copyLink(): void {
  void copy(target.value.url);
}
</script>

<template>
  <HostViewFrame
    :title="title"
    :ref-text="`${target.repo} ${target.host.changeRequestShort}${target.number}`"
    :fetching="query.isFetching.value"
    :pending="query.isPending.value"
    :failure="failure"
    :copy-label="t('space.ade.link.copy.tooltip', { label: target.host.changeRequestShort })"
    :open-label="t('space.ade.board.taskMenu.openHost.label', { label: target.host.label })"
    @refresh="refresh"
    @copy="copyLink"
    @open="openOnHost"
  >
    <template #badge>
      <Badge
        v-if="detail"
        :variant="TONE_BADGE[changeRequestTone(detail.state)]"
        class="shrink-0"
        data-testid="host-view-state"
      >
        {{ stateLabel }}
      </Badge>
    </template>
    <div v-if="detail" class="flex min-h-0 flex-1" data-testid="host-view-body">
      <main class="min-w-0 flex-1 overflow-y-auto p-3">
        <div
          class="mb-3 flex flex-wrap items-center gap-x-2 text-kira-sm text-muted-foreground"
          data-testid="host-view-meta"
        >
          <CodiconIcon name="git-pull-request" :size="13" />
          <span>{{
            t('space.hostView.header.opened.label', {
              author: detail.author.name || detail.author.login,
              when: formatRelative(detail.createdAt),
            })
          }}</span>
          <span class="font-data" data-testid="host-view-branches">{{
            t('space.hostView.header.branches.label', {
              head: detail.headRef,
              base: detail.baseRef,
            })
          }}</span>
        </div>
        <MarkdownBody :source="detail.body" />
        <HostViewConversation
          :threads="detail.threads"
          :truncated="!!detail.threadsTruncated"
          :compose="compose"
          :query-key="queryKey"
          :provider="target.provider"
        />
        <section v-if="compose" class="mt-4 flex flex-col gap-2" data-testid="host-view-compose">
          <SectionHeading :label="t('space.hostView.compose.heading.title')" />
          <CommentComposer
            kind="changeRequest"
            :compose="compose"
            :query-key="queryKey"
            :provider="target.provider"
          />
        </section>
      </main>
      <HostViewSidebar :detail="detail" />
    </div>
  </HostViewFrame>
</template>
