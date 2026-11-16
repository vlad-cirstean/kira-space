<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeTip from '../AdeTip.vue';
import { PR_TONE, prStateLabel } from '../panel/prState';
import { useOpenBranchPr } from '../panel/useOpenBranchPr';
import { useChangeRequests } from '../queries';
import { TONE_TEXT_CLASS } from '../tones';
import { CHECKS_GLYPH, PR_GLYPH } from './glyphs';
import type { BranchRowModel } from './usePlanModel';

// The branch's change request on its board row: a state glyph that opens it (and marks its update
// seen), plus the checks glyph. Reads the shared change-request cache; renders nothing without one.
const props = defineProps<{ row: BranchRowModel }>();

const crs = useChangeRequests();
const openPr = useOpenBranchPr();
const pr = computed(() => crs.data.value?.branches[props.row.id]);
const checks = computed(() => (pr.value ? CHECKS_GLYPH[pr.value.checksState] : undefined));
</script>

<template>
  <template v-if="pr">
    <TooltipIconButton
      :icon="PR_GLYPH[pr.state] ?? 'git-pull-request'"
      :label="pr.title"
      :aria-label="t('space.ade.plan.card.open.aria', { title: pr.title })"
      size="inline"
      class="size-4.5 shrink-0"
      :class="TONE_TEXT_CLASS[PR_TONE[pr.state] ?? 'grey']"
      :data-pr-state="pr.state"
      data-testid="ade-branch-pr"
      @click.stop="openPr.open(row.branch.taskId, row.id, pr.url)"
    >
      <div class="font-medium">{{ pr.title }}</div>
      <div>
        {{ prStateLabel(pr.state)
        }}<template v-if="pr.baseRef">
          · {{ t('space.ade.mainTab.prBase.label', { base: pr.baseRef }) }}</template
        >
      </div>
    </TooltipIconButton>
    <AdeTip v-if="checks" :text="t(checks.tip)">
      <span
        role="img"
        class="inline-flex shrink-0"
        :class="checks.class"
        :aria-label="t(checks.tip)"
        data-testid="ade-branch-pr-checks"
      >
        <CodiconIcon :name="checks.icon" :size="13" />
      </span>
    </AdeTip>
  </template>
</template>
