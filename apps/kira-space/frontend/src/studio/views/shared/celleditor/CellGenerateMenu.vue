<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { ref } from 'vue';
import { GENERATORS, type Generator } from './generate';

defineProps<{ disabled: boolean }>();
const emit = defineEmits<{ apply: [gen: Generator] }>();

const open = ref(false);
const anchorRef = ref<HTMLElement | null>(null);

function apply(gen: Generator): void {
  emit('apply', gen);
  open.value = false;
}
</script>

<template>
  <Popover :open="open" @update:open="open = $event">
    <span ref="anchorRef" class="relative shrink-0">
      <TooltipIconButton
        icon="sparkle"
        :label="t('studio.cellEditor.generate.tooltip')"
        disabled-trigger
        :disabled="disabled"
        data-testid="cell-editor-generate"
        @click="open = !open"
      />
      <PopoverAnchor :reference="anchorRef ?? undefined" />
    </span>
    <PopoverContent align="start" class="w-56 gap-0 p-0" data-testid="cell-editor-generate-popover">
      <div class="flex flex-col gap-px p-1">
        <Tooltip v-for="gen in GENERATORS" :key="gen.id">
          <TooltipTrigger as-child>
            <button
              type="button"
              :class="cn(rowVariants(), 'w-full border-0 bg-transparent text-left font-ui')"
              :data-testid="`cell-editor-generate-${gen.id}`"
              @click="apply(gen)"
            >
              {{ t(gen.labelKey) }}
            </button>
          </TooltipTrigger>
          <TooltipContent>{{ t(gen.hintKey) }}</TooltipContent>
        </Tooltip>
      </div>
    </PopoverContent>
  </Popover>
</template>
