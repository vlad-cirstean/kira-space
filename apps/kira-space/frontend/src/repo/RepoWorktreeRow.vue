<script setup lang="ts">
import type { WorktreeEntry } from '@kira/git-ipc';
import type { RepoSummary } from '@shared/domain/repo';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';
import { useTerminalsStore } from '../state/terminals';
import { worktreeLabel } from './state/worktrees';

// One nested worktree row of the Repos tab; the list owns navigation, so focus, click and menu
// go up as events.
const props = defineProps<{
  wt: WorktreeEntry;
  color: RepoSummary['color'];
  active: boolean;
  dimmed: boolean;
  navKey: string;
  tabindex: 0 | -1;
}>();
const emit = defineEmits<{ focus: []; click: []; menu: [event: MouseEvent] }>();

const terminalsStore = useTerminalsStore();
const rowClass = computed(() => [
  cn(rowVariants({ layout: 'tree', selected: props.active, muted: props.dimmed }), 'h-row'),
  { active: props.active },
]);
</script>

<template>
  <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter, Space and the arrows come from useListNavigation on the list. -->
  <div
    :class="rowClass"
    :style="rowIndent(1)"
    data-testid="repo-worktree-row"
    :data-worktree-path="wt.path"
    role="option"
    :data-nav-key="navKey"
    :tabindex="tabindex"
    :aria-selected="active"
    @focus="emit('focus')"
    @click.stop="emit('click')"
    @contextmenu.prevent.stop="emit('menu', $event)"
  >
    <span
      :class="colorMarkClass('rail', color)"
      data-testid="repo-worktree-rail"
      aria-hidden="true"
    />
    <CodiconIcon name="git-branch" :size="13" class="shrink-0" />
    <span class="min-w-0 flex-1"
      ><TruncatedText :text="wt.path">{{ worktreeLabel(wt) }}</TruncatedText></span
    >
    <Tooltip v-if="wt.isMain">
      <TooltipTrigger as-child>
        <span class="worktree-badge shrink-0 text-kira-sm text-subtle">{{
          t('common.main.label')
        }}</span>
      </TooltipTrigger>
      <TooltipContent>{{ t('common.mainWorktree.tooltip') }}</TooltipContent>
    </Tooltip>
    <Tooltip v-if="terminalsStore.terminalCountAtPath(wt.path) > 0">
      <TooltipTrigger as-child>
        <CodiconIcon
          name="terminal-bash"
          :size="12"
          class="shrink-0 text-subtle"
          data-testid="repo-terminal-indicator"
        />
      </TooltipTrigger>
      <TooltipContent>{{
        t('space.repo.panel.terminalOpen.tooltip', {
          count: terminalsStore.terminalCountAtPath(wt.path),
        })
      }}</TooltipContent>
    </Tooltip>
    <Tooltip v-if="wt.locked">
      <TooltipTrigger as-child>
        <CodiconIcon name="lock" :size="12" class="shrink-0 text-subtle" />
      </TooltipTrigger>
      <TooltipContent>{{ wt.locked?.reason }}</TooltipContent>
    </Tooltip>
  </div>
</template>
