<script setup lang="ts">
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useOpenBranchPr } from '../panel/useOpenBranchPr';
import PrGlyph from '../pr/PrGlyph.vue';
import { prStatusText } from '../pr/prGlyph';
import { useChangeRequests } from '../queries';
import type { BranchRowModel } from './usePlanModel';

// The branch's change request on its board row: the PR glyph, which opens it (and marks its update
// seen). Reads the shared change-request cache; renders nothing without one.
const props = defineProps<{ row: BranchRowModel }>();

const crs = useChangeRequests();
const openPr = useOpenBranchPr();
const pr = computed(() => crs.data.value?.branches[props.row.id]);
</script>

<template>
  <PrGlyph
    v-if="pr"
    variant="button"
    :status="pr"
    :title="pr.title"
    :base-ref="pr.baseRef"
    :aria-label="
      t('space.ade.plan.card.openPr.aria', { title: pr.title, status: prStatusText(pr) })
    "
    testid="ade-branch-pr"
    @open="openPr.open(row.branch.taskId, row.id, pr.url)"
  />
</template>
