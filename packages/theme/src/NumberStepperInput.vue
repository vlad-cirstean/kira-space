<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@theme/components/ui/input-group';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';
import { ref, useAttrs } from 'vue';

// Today's InputGroup + InputGroupInput type="number" + two Tooltip-wrapped
// InputGroupButton steppers recipe, carried once instead of 14 times. Owns the container ref and
// the stepUp/stepDown + synthetic input/change dispatch that used to live in the now-deleted
// useNumberStepper.ts composable.
defineOptions({ inheritAttrs: false });

const props = defineProps<{
  modelValue: string;
  groupClass?: HTMLAttributes['class'];
  inputClass?: HTMLAttributes['class'];
}>();

// $attrs (testid, @input/@update:model-value, aria-invalid, min/max, id, ...) bind to the inner
// input. `disabled` additionally reaches the step buttons directly -- stepBy's own el.disabled
// guard already no-ops a disabled field, but the buttons themselves should show it too.
const attrs = useAttrs();

const containerRef = ref<HTMLElement | null>(null);

function stepBy(dir: 1 | -1): void {
  const el = containerRef.value?.querySelector('input');
  if (!el || el.disabled) return;
  if (dir > 0) el.stepUp();
  else el.stepDown();
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}
</script>

<template>
  <div ref="containerRef" class="contents">
    <InputGroup
      variant="default"
      :class="cn('h-control-lg w-full rounded-kira-sm border-border bg-field', props.groupClass)"
    >
      <InputGroupInput
        type="number"
        :model-value="props.modelValue"
        v-bind="attrs"
        :class="
          cn(
            'h-full font-data [&::-webkit-inner-spin-button]:m-0 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:m-0 [&::-webkit-outer-spin-button]:appearance-none',
            props.inputClass,
          )
        "
      />
      <InputGroupAddon align="inline-end" class="self-stretch flex-col gap-0 p-0">
        <Tooltip>
          <TooltipTrigger as-child>
            <InputGroupButton
              class="flex-1 h-auto min-h-0 w-5 rounded-none p-0"
              tabindex="-1"
              aria-hidden="true"
              data-testid="number-step-up"
              :disabled="!!attrs.disabled"
              @mousedown.prevent="stepBy(1)"
            >
              <CodiconIcon name="chevron-up" :size="12" />
            </InputGroupButton>
          </TooltipTrigger>
          <TooltipContent>{{ t('theme.numberStepper.increase.tooltip') }}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger as-child>
            <InputGroupButton
              class="flex-1 h-auto min-h-0 w-5 rounded-none p-0"
              tabindex="-1"
              aria-hidden="true"
              data-testid="number-step-down"
              :disabled="!!attrs.disabled"
              @mousedown.prevent="stepBy(-1)"
            >
              <CodiconIcon name="chevron-down" :size="12" />
            </InputGroupButton>
          </TooltipTrigger>
          <TooltipContent>{{ t('theme.numberStepper.decrease.tooltip') }}</TooltipContent>
        </Tooltip>
      </InputGroupAddon>
    </InputGroup>
  </div>
</template>
