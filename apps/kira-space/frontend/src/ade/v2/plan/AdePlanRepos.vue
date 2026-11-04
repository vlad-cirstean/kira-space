<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import type { Ripple } from '../board/timeline';
import AdeRepoChip from './AdeRepoChip.vue';

// Sticky top of the Plan: one chip per repo, the "On merge" line of the selection, then Refresh all.
export interface RepoChipModel {
  codeRepoId: string;
  label: string;
  remote: string;
  lastFetchAt: number | null;
  shown: boolean;
  pending: boolean;
  error: string;
  summary: string;
}

defineProps<{ repos: RepoChipModel[]; allPending: boolean; ripple: Ripple | null }>();
const emit = defineEmits<{
  refreshAll: [];
  refreshOne: [codeRepoId: string];
  toggle: [codeRepoId: string];
}>();
</script>

<template>
  <div class="flex flex-wrap items-center gap-1.5 px-3 py-1.5" data-testid="ade-plan-repos">
    <AdeRepoChip
      v-for="r in repos"
      :key="r.codeRepoId"
      v-bind="r"
      @toggle="emit('toggle', r.codeRepoId)"
      @refresh="emit('refreshOne', r.codeRepoId)"
    />
    <span v-if="ripple" class="flex items-center gap-1.5 text-kira-sm" data-testid="ade-ripple">
      <span class="text-muted-foreground">{{ t('space.ade.plan.header.onMerge.label') }}</span>
      <span :class="ripple.tone === 'amber' ? 'text-tone-amber' : 'text-subtle'">{{
        ripple.text
      }}</span>
    </span>
    <Button
      class="ml-auto"
      variant="dialog"
      size="kira-lg"
      :pending="allPending"
      data-testid="ade-refresh-all"
      @click="emit('refreshAll')"
    >
      <CodiconIcon name="refresh" :size="12" />
      {{
        allPending ? t('space.ade.plan.header.refreshing.loading') : t('common.refreshAll.label')
      }}
    </Button>
  </div>
</template>
