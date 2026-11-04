<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Toggle } from '@theme/components/ui/toggle';
import { t } from '@theme/i18n';
import { useRelativeTime } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { useRepoLinksStore } from '../../../repo/state/repoLinks';
import { repoTint } from '../../../state/coderepos';
import AdeTip from '../AdeTip.vue';

// One repo of the plan: name toggles its branches on and off, `↻` fetches just this repo.
const props = defineProps<{
  codeRepoId: string;
  label: string;
  remote: string;
  lastFetchAt: number | null;
  shown: boolean;
  pending: boolean;
  error: string;
  /** What the last fetch changed, e.g. `3 refs changed`; `''` before any fetch. */
  summary: string;
}>();
const emit = defineEmits<{ toggle: []; refresh: [] }>();

const ago = useRelativeTime(() => props.lastFetchAt ?? 0);
const note = computed(() => {
  if (props.pending) return t('common.fetching.loading');
  if (props.error) return props.error;
  if (props.remote === '') {
    const none = t('space.ade.plan.repoChip.noRemote.status');
    return props.summary ? t('common.dotPair.label', { first: none, second: props.summary }) : none;
  }
  if (props.lastFetchAt === null) return t('space.ade.plan.repoChip.neverFetched.status');
  return props.summary
    ? t('common.dotPair.label', { first: String(ago.value), second: props.summary })
    : String(ago.value);
});
const repoLinks = useRepoLinksStore();
const tint = computed(() => repoTint(repoLinks.repoColorOf(props.codeRepoId)));
</script>

<template>
  <Badge
    variant="chip"
    class="h-control-lg max-w-80 gap-1.5 border border-border px-0.5"
    :class="shown ? 'bg-elevated' : 'bg-transparent'"
    data-testid="ade-repo-chip"
    :data-repo-id="codeRepoId"
  >
    <AdeTip :text="t('space.ade.plan.repoChip.toggle.tooltip')">
      <Toggle
        size="kira"
        class="shrink-0 px-1.5 font-medium"
        :class="tint.class"
        :style="tint.style"
        :model-value="shown"
        data-testid="ade-repo-toggle"
        @update:model-value="emit('toggle')"
      >
        {{ label }}
      </Toggle>
    </AdeTip>
    <span
      class="min-w-0 truncate text-kira-sm"
      :class="error ? 'text-tone-red' : 'text-subtle'"
      data-testid="ade-repo-note"
      >{{
        note
      }}</span
    >
    <AdeTip
      :text="
        remote === ''
          ? t('space.ade.plan.repoChip.rescan.tooltip', { label })
          : t('common.fetchTarget.tooltip', { name: label })
      "
    >
      <Button
        variant="toolbar"
        size="kira-icon"
        :pending="pending"
        :aria-label="t('space.ade.plan.repoChip.refresh.aria', { label })"
        data-testid="ade-repo-refresh"
        @click="emit('refresh')"
      >
        <CodiconIcon name="refresh" :size="12" />
      </Button>
    </AdeTip>
  </Badge>
</template>
