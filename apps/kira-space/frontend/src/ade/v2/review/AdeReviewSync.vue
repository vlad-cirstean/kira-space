<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useRelativeTime } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { useChangeRequests } from '../readQueries';
import type { RefSync, ReviewWindowTarget } from '../wire';
import { useReviewSync } from './useReviewSync';

// Freshness of the review's branch and base: Fetch, Update for a local ref behind its remote twin,
// and a status for refs that are ahead or diverged. Update only ever fast-forwards.
const props = defineProps<{ target: ReviewWindowTarget }>();
const { sync, fetching, updating, error, fetchNow, updateNow } = useReviewSync(() => props.target);

const ago = useRelativeTime(() => sync.data.value?.lastFetchAt ?? 0);
const fetchTip = computed(() => {
  const s = sync.data.value;
  if (!s) return '';
  return s.lastFetchAt === null
    ? t('space.ade.review.sync.fetchNever.tooltip', { remote: s.remote })
    : t('space.ade.review.sync.fetch.tooltip', { remote: s.remote, ago: String(ago.value) });
});

function updateTip(side: RefSync): string {
  return side.worktree === ''
    ? t('space.ade.review.sync.update.tooltip', { name: side.name, twin: side.twin })
    : t('space.ade.review.sync.updateWorktree.tooltip', {
        name: side.name,
        twin: side.twin,
        path: side.worktree,
      });
}
const divergedHint = (side: RefSync) =>
  t('space.ade.review.sync.diverged.hint', { name: side.name, twin: side.twin });

const changeRequests = useChangeRequests();
// The host compares with the change request's own target; the review follows the board's base.
const prBase = computed(() => {
  const cr = changeRequests.data.value?.branches[props.target.branchId];
  const base = sync.data.value?.base;
  if (!cr || !base || cr.state !== 'open' || cr.baseRef === '' || cr.baseRef === base.short) {
    return null;
  }
  return {
    label: t('space.ade.review.sync.prBase.label', { short: base.short, prBase: cr.baseRef }),
    hint: t('space.ade.review.sync.prBase.hint', { base: base.name }),
  };
});
</script>

<template>
  <div
    v-if="sync.data.value"
    class="wails-no-drag flex min-w-0 shrink-0 items-center gap-1.5"
    data-testid="ade-review-sync"
  >
    <TooltipIconButton
      v-if="sync.data.value.remote !== ''"
      icon="refresh"
      :label="fetchTip"
      :aria-label="t('common.fetchTarget.tooltip', { name: sync.data.value.remote })"
      :icon-size="12"
      variant="title"
      size="title"
      :pending="fetching"
      data-testid="ade-review-sync-fetch"
      @click="fetchNow()"
    />
    <span v-if="fetching" class="shrink-0 text-kira-sm text-subtle">{{
      t('common.fetching.loading')
    }}</span>
    <Tooltip v-if="sync.data.value.branch.state === 'behind'">
      <TooltipTrigger as-child>
        <Button
          variant="title"
          size="title-labelled"
          class="shrink-0"
          :pending="updating"
          data-testid="ade-review-sync-update-branch"
          @click="updateNow('branch')"
        >
          <CodiconIcon name="arrow-down" :size="12" />
          {{
            t('space.ade.review.sync.updateBranch.label', { count: sync.data.value.branch.behind })
          }}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{{ updateTip(sync.data.value.branch) }}</TooltipContent>
    </Tooltip>
    <Tooltip v-if="sync.data.value.base.state === 'behind'">
      <TooltipTrigger as-child>
        <Button
          variant="title"
          size="title-labelled"
          class="shrink-0"
          :pending="updating"
          data-testid="ade-review-sync-update-base"
          @click="updateNow('base')"
        >
          <CodiconIcon name="arrow-down" :size="12" />
          {{ t('space.ade.review.sync.updateBase.label', { count: sync.data.value.base.behind }) }}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{{ updateTip(sync.data.value.base) }}</TooltipContent>
    </Tooltip>
    <Badge
      v-if="sync.data.value.branch.state === 'ahead'"
      class="shrink-0"
      :data-state="sync.data.value.branch.state"
      data-testid="ade-review-sync-branch-state"
    >
      {{ t('space.ade.review.sync.ahead.status', { count: sync.data.value.branch.ahead }) }}
    </Badge>
    <Tooltip v-if="sync.data.value.branch.state === 'diverged'">
      <TooltipTrigger as-child>
        <Badge
          variant="warn"
          tabindex="0"
          class="shrink-0"
          :aria-label="divergedHint(sync.data.value.branch)"
          :data-state="sync.data.value.branch.state"
          data-testid="ade-review-sync-branch-state"
        >
          {{ t('space.ade.review.sync.diverged.status') }}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{{ divergedHint(sync.data.value.branch) }}</TooltipContent>
    </Tooltip>
    <Tooltip v-if="sync.data.value.base.state === 'diverged'">
      <TooltipTrigger as-child>
        <Badge
          variant="warn"
          tabindex="0"
          class="shrink-0"
          :aria-label="divergedHint(sync.data.value.base)"
          :data-state="sync.data.value.base.state"
          data-testid="ade-review-sync-base-state"
        >
          {{ t('space.ade.review.sync.diverged.status') }}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{{ divergedHint(sync.data.value.base) }}</TooltipContent>
    </Tooltip>
    <Tooltip v-if="prBase">
      <TooltipTrigger as-child>
        <Badge
          variant="warn"
          tabindex="0"
          class="shrink-0"
          :aria-label="prBase.hint"
          data-testid="ade-review-sync-pr-base"
        >
          {{ prBase.label }}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{{ prBase.hint }}</TooltipContent>
    </Tooltip>
    <span
      v-if="error"
      class="flex min-w-0 max-w-64 text-kira-sm text-tone-red"
      data-testid="ade-review-sync-error"
      ><TruncatedText :text="error" /></span
    >
  </div>
</template>
