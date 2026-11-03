<script setup lang="ts">
import type { ScriptRun } from '@shared/domain/scriptRuns';
import type { CustomScript, ScriptFileProblem } from '@shared/domain/scripts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import RunElapsed from './runs/RunElapsed.vue';
import ScriptScheduleLine from './schedule/ScriptScheduleLine.vue';
import SmartBadge from './smart/SmartBadge.vue';

defineOptions({ inheritAttrs: false });
defineProps<{
  script: CustomScript;
  depth: 0 | 1;
  liveRun?: ScriptRun;
  /** The run's file has an error now; the last good version keeps running. */
  problem?: ScriptFileProblem | null;
  selected: boolean;
  /** The script's own tab is still running: the play button shows it instead of starting. */
  live: boolean;
}>();
const emit = defineEmits<{ run: []; open: []; menu: [event: MouseEvent] }>();

function problemText(p: ScriptFileProblem): string {
  return p.line > 0
    ? t('common.config.lineError.error', { line: p.line, text: p.message })
    : t('common.config.fileError.error', { text: p.message });
}

function firstLine(command: string): string {
  const [first = '', ...rest] = command.split('\n');
  return rest.length > 0 ? `${first}…` : first;
}
</script>

<template>
  <Tooltip>
    <TooltipTrigger as-child>
      <!-- biome-ignore lint/a11y/useFocusableInteractive: tabindex arrives in $attrs from useListNavigation.rowProps. -->
      <div
        v-bind="$attrs"
        role="treeitem"
        :aria-selected="selected"
        :class="cn(rowVariants({ layout: 'double', selected }), 'w-full items-center text-left')"
        :style="rowIndent(depth)"
        :data-testid="`script-${script.id}`"
        @dblclick="emit('open')"
        @contextmenu.prevent.stop="emit('menu', $event)"
      >
        <span
          :class="colorMarkClass('rail', script.color)"
          data-testid="script-rail"
          aria-hidden="true"
        />
        <div class="flex-1 min-w-0 flex flex-col">
          <span class="flex min-w-0 items-center gap-1">
            <SmartBadge v-if="script.kind === 'smart'" />
            <span class="truncate">{{ script.name }}</span>
            <Tooltip v-if="problem">
              <TooltipTrigger as-child>
                <CodiconIcon
                  name="warning"
                  :size="12"
                  class="shrink-0 text-tone-red"
                  data-testid="script-row-problem"
                />
              </TooltipTrigger>
              <TooltipContent>{{ problemText(problem) }}</TooltipContent>
            </Tooltip>
          </span>
          <span class="truncate text-muted-foreground text-kira-sm">{{
            firstLine(script.command)
          }}</span>
          <ScriptScheduleLine v-if="script.schedule" :script="script" />
        </div>
        <RunElapsed
          v-if="liveRun"
          :run="liveRun"
          class="shrink-0 text-kira-sm text-muted-foreground"
        />
        <span class="flex shrink-0 items-center justify-center" data-testid="script-run-slot">
          <Spinner v-if="liveRun" label="" data-testid="script-running" />
          <TooltipIconButton
            icon="play"
            :label="live ? t('automations.script.showRun.tooltip') : t('common.action.run.label')"
            tabindex="-1"
            data-testid="script-play"
            @click.stop="emit('run')"
            @dblclick.stop
          />
        </span>
      </div>
    </TooltipTrigger>
    <TooltipContent class="whitespace-pre-wrap">{{ script.command }}</TooltipContent>
  </Tooltip>
</template>
