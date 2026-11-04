<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Separator } from '@theme/components/ui/separator';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import { tagLabel } from '../board/actions';
import type { BranchRowModel } from './usePlanModel';
import { usePlanModel } from './usePlanModel';

// One rich tooltip per branch: identity, derived tag in its tone, base, progress, merged/deployed.
const props = defineProps<{ row: BranchRowModel; color: string | null }>();
const open = defineModel<boolean>('open');
const { liveNow } = usePlanModel();

const label = computed(() => tagLabel(props.row.tag, liveNow.value.getTime()));
const tagParts = computed(() => props.row.tag.tipParts ?? [props.row.tag.tip]);
const hasDetails = computed(
  () =>
    props.row.base !== null ||
    props.row.prog !== null ||
    props.row.chips.merged.length + props.row.chips.deployed.length > 0,
);
const SEG: Record<string, string> = {
  done: 'bg-tone-green-solid',
  running: 'bg-tone-amber-solid',
  bad: 'bg-tone-red-solid',
};
const segClass = (state: string): string => SEG[state] ?? 'bg-border';
const staleClass = (tone: string): string => (tone === 'stale' ? 'text-tone-amber' : '');
</script>

<template>
  <Tooltip v-model:open="open">
    <TooltipTrigger as-child>
      <slot />
    </TooltipTrigger>
    <TooltipContent variant="surface" side="bottom" align="start" data-testid="ade-branch-tip">
      <div class="flex items-center gap-1.5">
        <span
          v-if="color"
          class="size-2.5 shrink-0 rounded-kira-sm"
          :style="{ background: color }"
        />
        <span class="min-w-0 font-data font-medium wrap-anywhere">{{ row.name }}</span>
      </div>
      <span v-if="row.context" class="block text-kira-sm text-muted-foreground">{{
        row.context
      }}</span>
      <div class="flex flex-wrap items-center gap-1">
        <AdeRepoTag :code-repo-id="row.branch.codeRepoId" :label="row.repo" />
        <AdeChip :label="label" :tone="row.tag.tone" />
        <AdeChip v-if="row.isReview" :label="row.branch.owner" tone="blue" />
      </div>
      <span v-if="row.isReview" class="block text-kira-sm text-muted-foreground">{{
        t('space.ade.plan.branchRow.review.tooltip')
      }}</span>
      <span class="block text-kira-sm text-muted-foreground"><VarText :parts="tagParts" /></span>
      <span
        v-if="row.branch.origin === 'agent'"
        class="flex items-center gap-1 text-kira-sm text-muted-foreground"
      >
        <CodiconIcon name="sparkle" :size="12" />{{ t('space.ade.agentOrigin.tooltip') }}
      </span>
      <Separator v-if="hasDetails" />
      <KeyValueList v-if="hasDetails" class="text-kira-sm">
        <template v-if="row.base">
          <dt>{{ t('space.common.base.label') }}</dt>
          <dd class="flex items-center gap-1">
            <span
              v-if="row.base.color"
              class="size-2 shrink-0 rounded-kira-sm"
              :style="{ background: row.base.color }"
            />
            <VarText :parts="row.base.tip" />
          </dd>
        </template>
        <template v-if="row.prog">
          <dt>{{ t('space.ade.plan.branchTip.progress.label') }}</dt>
          <dd class="flex flex-wrap items-center gap-1">
            <span class="inline-flex gap-0.5">
              <span
                v-for="(seg, i) in row.prog.segs"
                :key="i"
                class="h-1.25 w-1.75 rounded-kira-sm"
                :class="segClass(seg)"
              />
            </span>
            <span>{{ row.prog.label }}</span>
            <span class="text-muted-foreground">{{ row.prog.tip }}</span>
          </dd>
        </template>
        <template v-if="row.chips.merged.length">
          <dt>{{ t('space.ade.branchPanel.mergedInto.label') }}</dt>
          <dd class="flex flex-col">
            <span v-for="c in row.chips.merged" :key="c.label" class="flex gap-1">
              <span class="shrink-0" :class="staleClass(c.tone)">{{ c.label }}</span>
              <span class="text-muted-foreground">{{ c.tip }}</span>
            </span>
          </dd>
        </template>
        <template v-if="row.chips.deployed.length">
          <dt>{{ t('space.ade.branchPanel.deployedTo.label') }}</dt>
          <dd class="flex flex-col">
            <span v-for="c in row.chips.deployed" :key="c.label" class="flex gap-1">
              <span class="shrink-0" :class="staleClass(c.tone)">{{ c.label }}</span>
              <span class="text-muted-foreground">{{ c.tip }}</span>
            </span>
          </dd>
        </template>
      </KeyValueList>
    </TooltipContent>
  </Tooltip>
</template>
