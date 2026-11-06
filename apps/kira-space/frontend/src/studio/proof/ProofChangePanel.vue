<script setup lang="ts">
import type { ProofDetail, ProofDiff } from '@shared/domain/proof';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Badge, type BadgeVariants } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';
import { useReviewEdit } from './queries';
import { useProofWindowStore } from './state';
import { proofTabId } from './tabHost';

// The pending edit: which revision to read, what changed step by step, and the accept/reject pair.
const props = defineProps<{ detail: ProofDetail; pending: NonNullable<ProofDetail['pending']> }>();
const store = useProofWindowStore();
const { accept, reject } = useReviewEdit();

const view = computed<'current' | 'proposed' | 'compare'>({
  get: () => {
    if (store.compare) return 'compare';
    return store.side === 'current' ? 'current' : 'proposed';
  },
  set: (v) => {
    store.compare = v === 'compare';
    store.side = v === 'current' ? 'current' : 'pending';
  },
});
const VIEWS = computed(() => [
  { value: 'current', label: t('studio.proof.change.current.label'), testid: 'proof-view-current' },
  {
    value: 'proposed',
    label: t('studio.proof.change.proposed.label'),
    testid: 'proof-view-proposed',
  },
  { value: 'compare', label: t('studio.http.history.compare.label'), testid: 'proof-view-compare' },
]);

const DIFF_LABEL: Record<ProofDiff, () => string> = {
  added: () => t('common.added.status'),
  changed: () => t('common.changed.status'),
  removed: () => t('common.removed.status'),
  unchanged: () => t('studio.http.diff.unchangedRow.status'),
};
const DIFF_VARIANT: Record<ProofDiff, BadgeVariants['variant']> = {
  added: 'ok',
  changed: 'warn',
  removed: 'error',
  unchanged: 'default',
};

const rows = computed(() => [
  ...props.pending.steps.map((meta) => ({ meta, rev: props.pending.rev })),
  ...props.detail.removed.map((meta) => ({ meta, rev: props.detail.current.rev })),
]);

function select(row: (typeof rows.value)[number]): void {
  if (row.meta.diff === 'removed') store.compare = true;
  store.side = 'pending';
  store.activeId = proofTabId(row.rev, row.meta.ordinal);
}

const locked = computed(() => accept.isPending.value || reject.isPending.value);
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="proof-change">
    <div
      class="flex h-tabbar shrink-0 items-center bg-chrome px-3 text-kira-sm font-medium text-muted-foreground"
    >
      {{ t('studio.proof.change.title') }}
    </div>
    <p
      v-if="pending.reason"
      class="m-0 shrink-0 whitespace-pre-wrap p-3"
      data-testid="proof-change-reason"
    >
      {{ pending.reason }}
    </p>
    <div class="shrink-0 p-2">
      <SecondaryTabs
        v-model="view"
        variant="segmented"
        :items="VIEWS"
        :aria-label="t('studio.proof.change.view.aria')"
        data-testid="proof-change-view"
      />
    </div>
    <ul
      class="m-0 min-h-0 flex-1 list-none overflow-y-auto p-1"
      :aria-label="t('studio.proof.change.steps.aria')"
    >
      <li v-for="row in rows" :key="proofTabId(row.rev, row.meta.ordinal)">
        <button
          type="button"
          :class="
            cn(
              rowVariants({
                layout: 'tree',
                selected: store.activeId === proofTabId(row.rev, row.meta.ordinal),
              }),
              'w-full gap-1.5 rounded-kira-sm border-0 bg-transparent pl-2 text-left',
            )
          "
          :data-testid="`proof-change-step-${row.meta.ordinal}`"
          :data-diff="row.meta.diff"
          @click="select(row)"
        >
          <span :class="cn(rowMeta, 'w-6 shrink-0')">{{ `#${row.meta.ordinal}` }}</span>
          <span class="min-w-0 flex-1 truncate">{{ row.meta.description }}</span>
          <Badge :variant="DIFF_VARIANT[row.meta.diff]">{{ DIFF_LABEL[row.meta.diff]() }}</Badge>
        </button>
      </li>
    </ul>
    <div class="flex shrink-0 justify-end gap-2 p-2">
      <Button
        variant="dialog"
        size="kira-lg"
        :pending="reject.isPending.value"
        :disabled="locked"
        data-testid="proof-reject"
        @click="reject.mutate(detail.id)"
      >
        <CodiconIcon name="close" :size="13" />
        {{ t('studio.proof.change.reject.label') }}
      </Button>
      <Button
        variant="dialog-primary"
        size="kira-lg"
        :pending="accept.isPending.value"
        :disabled="locked"
        data-testid="proof-accept"
        @click="accept.mutate(detail.id)"
      >
        <CodiconIcon name="check" :size="13" />
        {{ t('studio.proof.change.accept.label') }}
      </Button>
    </div>
  </div>
</template>
