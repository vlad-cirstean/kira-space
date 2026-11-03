<script setup lang="ts">
import type { ScriptFileProblem } from '@shared/domain/scripts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';

// A run file that is invalid and has no earlier valid version: listed so it can be opened and fixed.
defineOptions({ inheritAttrs: false });
defineProps<{ problem: ScriptFileProblem; selected: boolean }>();
const emit = defineEmits<{ open: [] }>();
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
        :style="rowIndent(0)"
        :data-testid="`script-broken-${problem.file}`"
        @dblclick="emit('open')"
      >
        <CodiconIcon name="warning" :size="13" class="shrink-0 text-tone-red" aria-hidden="true" />
        <div class="flex-1 min-w-0 flex flex-col">
          <TruncatedText :text="problem.file" />
          <span class="truncate text-kira-sm text-tone-red">{{
            t('automations.file.broken.status')
          }}</span>
        </div>
      </div>
    </TooltipTrigger>
    <TooltipContent>{{
      problem.line > 0
        ? t('common.config.lineError.error', { line: problem.line, text: problem.message })
        : t('common.config.fileError.error', { text: problem.message })
    }}</TooltipContent>
  </Tooltip>
</template>
