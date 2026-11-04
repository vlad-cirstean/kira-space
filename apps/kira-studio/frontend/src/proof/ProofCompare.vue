<script setup lang="ts">
import type { ProofDetail, ProofStepMeta } from '@shared/domain/proof';
import EmptyState from '@theme/components/EmptyState.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import ProofStepView from './ProofStepView.vue';

// Old (current revision) beside New (pending revision) for one step. An added step has no old
// side; a removed step has no new side.
const props = defineProps<{ detail: ProofDetail; meta: ProofStepMeta }>();

const oldOrdinal = computed(() =>
  props.meta.diff === 'removed' ? props.meta.ordinal : props.meta.originOrdinal,
);
const hasNew = computed(() => props.meta.diff !== 'removed');
</script>

<template>
  <div class="flex h-full min-h-0 gap-0.5 bg-chrome" data-testid="proof-compare">
    <section
      class="flex min-h-0 min-w-0 flex-1 flex-col bg-bg"
      :aria-label="t('studio.proof.compare.old.label')"
      data-testid="proof-compare-old"
    >
      <div class="flex h-control shrink-0 items-center px-3">
        <Badge>{{ t('studio.proof.compare.old.label') }}</Badge>
      </div>
      <div class="min-h-0 flex-1">
        <ProofStepView
          v-if="oldOrdinal !== null"
          :key="`${detail.current.rev}:${oldOrdinal}`"
          :session-id="detail.id"
          :rev="detail.current.rev"
          :ordinal="oldOrdinal"
        />
        <EmptyState
          v-else
          fill
          icon="diff-added"
          :title="t('studio.proof.compare.added.emptyHint')"
        />
      </div>
    </section>
    <section
      class="flex min-h-0 min-w-0 flex-1 flex-col bg-bg"
      :aria-label="t('studio.proof.compare.new.label')"
      data-testid="proof-compare-new"
    >
      <div class="flex h-control shrink-0 items-center px-3">
        <Badge variant="warn">{{ t('studio.proof.compare.new.label') }}</Badge>
      </div>
      <div class="min-h-0 flex-1">
        <ProofStepView
          v-if="hasNew && detail.pending"
          :key="`${detail.pending.rev}:${meta.ordinal}`"
          :session-id="detail.id"
          :rev="detail.pending.rev"
          :ordinal="meta.ordinal"
        />
        <EmptyState
          v-else
          fill
          icon="diff-removed"
          :title="t('studio.proof.compare.removed.emptyHint')"
        />
      </div>
    </section>
  </div>
</template>
