<script setup lang="ts">
import type { ProofDiagramPayload } from '@shared/domain/proof';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { computedAsync, useScroll } from '@vueuse/core';
import { errorText } from '@workbench/bridge/codedError';
import { nextTick, ref, useTemplateRef, watch } from 'vue';
import { useChartTheme } from '../charts/theme';
import MonacoHost from '../editor/MonacoHost.vue';
import { DiagramTooLargeError, renderDiagram } from './diagramRender';

// One Mermaid step: sanitized SVG in a scroller, or the parser message above the source.
const props = defineProps<{
  payload: ProofDiagramPayload;
  description: string;
  seed: { top: number; left: number };
  /** `pane` fills its parent; `content` takes the height of the drawing (the Details tab). */
  fit?: 'pane' | 'content';
}>();
const emit = defineEmits<{ 'update:scroll': [top: number, left: number]; ready: [] }>();

const theme = useChartTheme();
const failure = ref<unknown>(null);
const evaluating = ref(true);
const svg = computedAsync(
  async () => {
    failure.value = null;
    return renderDiagram(props.payload.source, theme.value);
  },
  null,
  { evaluating, onError: (err) => (failure.value = err) },
);

const scroller = useTemplateRef<HTMLElement>('scroller');
const { x, y } = useScroll(scroller);
watch([x, y], ([left, top]) => emit('update:scroll', top, left));

let announced = false;
function announce(): void {
  if (announced) return;
  announced = true;
  emit('ready');
}

watch(svg, async (value) => {
  if (!value) return;
  await nextTick();
  x.value = props.seed.left;
  y.value = props.seed.top;
  announce();
});
watch(failure, (err) => {
  if (err) announce();
});
</script>

<template>
  <div
    v-if="failure"
    :class="cn('flex min-h-0 flex-col', fit !== 'content' && 'size-full')"
    data-testid="proof-diagram-error"
  >
    <EmptyState
      icon="warning"
      :title="t('studio.proof.diagram.invalid.title')"
      :description="
        failure instanceof DiagramTooLargeError
          ? t('studio.proof.diagram.tooLarge.body')
          : errorText(failure)
      "
    />
    <div
      :class="fit === 'content' ? 'h-48' : 'min-h-0 flex-1'"
      :aria-label="t('studio.proof.diagram.source.label')"
      data-testid="proof-diagram-source"
    >
      <MonacoHost :doc="payload.source" language="plain" readonly force-wrap />
    </div>
  </div>
  <figure
    v-else
    class="m-0 min-h-0"
    :class="fit !== 'content' && 'size-full'"
    :aria-label="t('studio.proof.diagram.region.aria', { description })"
    data-testid="proof-diagram"
    :data-diagram-type="payload.type"
  >
    <!-- svg is sanitized twice: mermaid strict mode, then a private DOMPurify pass (diagramRender.ts). -->
    <div
      ref="scroller"
      :class="
        cn('select-text p-3', fit === 'content' ? 'overflow-x-auto' : 'size-full overflow-auto')
      "
      v-html="svg"
    ></div>
  </figure>
</template>
