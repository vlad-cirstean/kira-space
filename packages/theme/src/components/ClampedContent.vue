<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useResizeObserver } from '@vueuse/core';
import { computed, ref, useId, useTemplateRef } from 'vue';

// Multi-line counterpart of TruncatedText: shows the first `lines` lines of long content under a
// fade, with a Show more / Show less link. Content that fits shows no link. The content stays in
// the DOM while clamped, so selection and copy still reach all of it.
const props = withDefaults(defineProps<{ lines?: number; toggleTestid?: string }>(), {
  lines: 6,
  toggleTestid: 'clamped-content-toggle',
});
const expanded = defineModel<boolean>('expanded', { default: false });

const id = useId();
const inner = useTemplateRef<HTMLElement>('inner');
const limit = ref(0);
const overflowing = ref(false);
const clamped = computed(() => overflowing.value && !expanded.value);

// Inline max-height: the limit derives from the measured line height, which Tailwind cannot know.
useResizeObserver(inner, () => {
  const node = inner.value;
  if (node === null) return;
  const style = getComputedStyle(node);
  const parsed = Number.parseFloat(style.lineHeight);
  const lineHeight = Number.isNaN(parsed) ? 1.5 * Number.parseFloat(style.fontSize) : parsed;
  limit.value = Math.round(props.lines * lineHeight);
  overflowing.value = node.scrollHeight > limit.value + 1;
});
</script>

<template>
  <div class="flex min-w-0 flex-col items-start gap-0.5">
    <div
      :id="id"
      class="w-full min-w-0"
      :class="clamped && 'overflow-hidden mask-b-from-60%'"
      :style="clamped ? { maxHeight: `${limit}px` } : undefined"
      :data-clamped="clamped"
      data-slot="clamped-content-box"
    >
      <div ref="inner"><slot /></div>
    </div>
    <Button
      v-if="overflowing"
      variant="link"
      size="inline"
      :aria-expanded="expanded"
      :aria-controls="id"
      :data-testid="toggleTestid"
      @click="expanded = !expanded"
      >{{
        expanded ? t('common.action.showLess.label') : t('common.action.showMore.label')
      }}</Button
    >
  </div>
</template>
