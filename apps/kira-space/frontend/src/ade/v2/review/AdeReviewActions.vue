<script setup lang="ts">
import type { ChangeRequestReviewEvent } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { hostWords, type MessageKey, t } from '@theme/i18n';
import { useTimeoutFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import type { ReviewWindowTarget } from '../wire';
import AdeReviewSubmitDialog from './AdeReviewSubmitDialog.vue';
import { reviewGate, useChangeRequestReview } from './useChangeRequestReview';

const props = defineProps<{ target: ReviewWindowTarget }>();

const { state, openChangeRequest } = useChangeRequestReview(() => props.target);
const data = computed(() => state.data.value);
const ok = computed(() => (data.value?.kind === 'ok' ? data.value : null));
const unavailable = computed(() => (data.value?.kind === 'unavailable' ? data.value : null));
const none = computed(() => data.value?.kind === 'none');
const failed = computed(() =>
  state.isError.value && !data.value ? errorText(state.error.value) : null,
);
const short = computed(() => hostWords(ok.value?.host).changeRequestShort);

const EVENTS: readonly { event: ChangeRequestReviewEvent; label: MessageKey }[] = [
  { event: 'comment', label: 'common.comment.label' },
  { event: 'approve', label: 'common.action.approve.label' },
  { event: 'requestChanges', label: 'space.ade.review.actions.requestChanges.label' },
];
const items = computed<MenuItem[]>(() => {
  const s = ok.value;
  if (!s) return [];
  return EVENTS.map((e) => {
    const gate = reviewGate(s, e.event);
    return {
      type: 'item',
      id: e.event,
      testId: `ade-review-action-${e.event}`,
      label: t(e.label),
      disabled: !gate.enabled,
      hint: gate.enabled ? undefined : gate.reason,
      run: () => {
        mode.value = e.event;
      },
    };
  });
});

const mode = ref<ChangeRequestReviewEvent | 'conversation' | null>(null);
const posted = ref<'review' | 'comment' | null>(null);
const clearPosted = useTimeoutFn(() => (posted.value = null), TIMING.postedStatus, {
  immediate: false,
});
function done(kind: 'review' | 'comment'): void {
  posted.value = kind;
  clearPosted.start();
}
</script>

<template>
  <div
    v-if="ok || unavailable || failed || none"
    class="wails-no-drag flex shrink-0 items-center gap-1"
    data-testid="ade-review-actions"
  >
    <Tooltip v-if="unavailable || failed || none">
      <TooltipTrigger as-child>
        <TooltipDisabledTrigger>
          <Button
            variant="title"
            size="title-labelled"
            disabled
            data-testid="ade-review-actions-trigger"
          >
            {{ t('space.ade.board.branchTag.review.label') }}
          </Button>
        </TooltipDisabledTrigger>
      </TooltipTrigger>
      <TooltipContent>{{
        none
          ? t('space.ade.review.actions.none.hint', { short })
          : unavailable
            ? unavailable.status.reason
            : t('space.ade.review.actions.stateFailed.hint', { reason: failed ?? '' })
      }}</TooltipContent>
    </Tooltip>
    <template v-else-if="ok">
      <span
        v-if="posted"
        role="status"
        class="flex items-center gap-1 text-kira-sm text-muted-foreground"
        data-testid="ade-review-posted"
      >
        {{
          posted === 'review'
            ? t('space.ade.review.actions.postedReview.status')
            : t('space.ade.review.actions.postedComment.status')
        }}
        <Button
          variant="link"
          size="inline"
          class="text-kira-sm"
          @click="openChangeRequest(ok.number)"
        >
          {{ t('space.ade.review.actions.open.label', { short }) }}
        </Button>
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <Button variant="title" size="title-labelled" data-testid="ade-review-actions-trigger">
            {{ t('space.ade.board.branchTag.review.label') }}
            <CodiconIcon name="chevron-down" :size="16" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <MenuItems :items="items" />
        </DropdownMenuContent>
      </DropdownMenu>
      <Button
        variant="title"
        size="title-labelled"
        data-testid="ade-review-cr-comment"
        @click="mode = 'conversation'"
      >
        <CodiconIcon name="comment" :size="16" />
        {{ t('space.ade.review.actions.conversation.label', { short }) }}
      </Button>
    </template>
    <AdeReviewSubmitDialog
      v-if="mode && ok"
      :mode="mode"
      :target="target"
      :review="ok"
      @done="done"
      @close="mode = null"
    />
  </div>
</template>
