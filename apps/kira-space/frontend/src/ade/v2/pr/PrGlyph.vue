<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { TONE_TEXT_CLASS } from '../tones';
import PrStatusLines from './PrStatusLines.vue';
import { type PrStatus, prGlyph, prStateLabel, prStatusText } from './prGlyph';

// The one pull request icon. `button` opens it (branch row); `tooltip` is a hover glyph (task panel
// line); `inline` shows the lines as text beside the glyph, where hover does not reach (host view
// sidebar, phone). The corner dot is the checks and pulses while they run.
defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    status: PrStatus;
    variant: 'button' | 'tooltip' | 'inline';
    /** Bold first tooltip line (button). */
    title?: string;
    /** Target branch, after the state. */
    baseRef?: string;
    /** Button accessible name; defaults to the title plus status. */
    ariaLabel?: string;
    testid?: string;
    /** Inline lines get `data-testid="<prefix>-<part>"`. */
    linePrefix?: string;
  }>(),
  {
    title: undefined,
    baseRef: undefined,
    ariaLabel: undefined,
    testid: undefined,
    linePrefix: undefined,
  },
);
const emit = defineEmits<{ open: [] }>();

const model = computed(() => prGlyph(props.status));
const text = computed(() => prStatusText(props.status));
const stateLine = computed(() => {
  const state = prStateLabel(props.status.state);
  return props.baseRef
    ? t('common.dotPair.label', {
        first: state,
        second: t('space.ade.mainTab.prBase.label', { base: props.baseRef }),
      })
    : state;
});
const toneClass = computed(() => TONE_TEXT_CLASS[model.value.tone]);
const attrs = computed(() => ({
  'data-testid': props.testid,
  'data-pr-state': props.status.state,
  'data-tone': model.value.tone,
  'data-checks': model.value.checks?.key,
  'data-review': model.value.review ?? undefined,
  'data-mergeable': model.value.mergeable ?? undefined,
  'data-auto-merge': model.value.autoMerge ? 'true' : undefined,
}));
</script>

<template>
  <TooltipIconButton
    v-if="variant === 'button'"
    :icon="model.icon"
    :label="title ?? text"
    :aria-label="ariaLabel ?? text"
    size="inline"
    class="relative size-4.5 shrink-0"
    :class="toneClass"
    v-bind="attrs"
    @click.stop="emit('open')"
  >
    <div v-if="title" class="font-medium">{{ title }}</div>
    <div>{{ stateLine }}</div>
    <PrStatusLines :lines="model.lines" as="div" />
    <template #badge>
      <StatusDot
        v-if="model.checks"
        class="absolute right-0 bottom-0"
        :tone="model.checks.dot"
        :pulse="model.checks.pending"
        aria-hidden="true"
      />
    </template>
  </TooltipIconButton>
  <Tooltip v-else-if="variant === 'tooltip'">
    <TooltipTrigger as-child>
      <span
        role="img"
        class="relative inline-flex size-4.5 shrink-0 items-center justify-center"
        :class="toneClass"
        :aria-label="text"
        v-bind="attrs"
      >
        <CodiconIcon :name="model.icon" :size="13" />
        <StatusDot
          v-if="model.checks"
          class="absolute right-0 bottom-0"
          :tone="model.checks.dot"
          :pulse="model.checks.pending"
          aria-hidden="true"
        />
      </span>
    </TooltipTrigger>
    <TooltipContent>
      <div>{{ stateLine }}</div>
      <PrStatusLines :lines="model.lines" as="div" />
    </TooltipContent>
  </Tooltip>
  <span v-else class="flex min-w-0 items-start gap-2 text-kira-sm" v-bind="attrs">
    <span
      class="relative inline-flex size-4.5 shrink-0 items-center justify-center"
      :class="toneClass"
      aria-hidden="true"
    >
      <CodiconIcon :name="model.icon" :size="13" />
      <StatusDot
        v-if="model.checks"
        class="absolute right-0 bottom-0"
        :tone="model.checks.dot"
        :pulse="model.checks.pending"
      />
    </span>
    <span class="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5">
      <span data-part="state">{{ stateLine }}</span>
      <PrStatusLines :lines="model.lines" tinted :line-prefix="linePrefix" />
    </span>
  </span>
</template>
