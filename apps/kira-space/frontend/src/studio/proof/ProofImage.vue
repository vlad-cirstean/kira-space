<script setup lang="ts">
import type { ProofImagePayload } from '@shared/domain/proof';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { useScroll } from '@vueuse/core';
import { nextTick, ref, useTemplateRef, watch } from 'vue';

// One JPEG step: the stored bytes as a data URL (the window CSP allows data:, not blob:), fitted
// to the width and never upscaled; a tall image scrolls.
const props = defineProps<{
  payload: ProofImagePayload;
  description: string;
  seed: { top: number; left: number };
}>();
const emit = defineEmits<{ 'update:scroll': [top: number, left: number]; ready: [] }>();

const failed = ref(false);
const scroller = useTemplateRef<HTMLElement>('scroller');
const { x, y } = useScroll(scroller);
let restored = false;
// Scrolls fired before the saved position is restored would overwrite it.
watch([x, y], ([left, top]) => {
  if (restored) emit('update:scroll', top, left);
});

let announced = false;
function announce(): void {
  if (announced) return;
  announced = true;
  emit('ready');
}

async function onLoad(): Promise<void> {
  await nextTick();
  x.value = props.seed.left;
  y.value = props.seed.top;
  await nextTick();
  restored = true;
  announce();
}

function onError(): void {
  failed.value = true;
  announce();
}
</script>

<template>
  <EmptyState
    v-if="failed"
    fill
    icon="warning"
    :title="t('studio.proof.image.invalid.title')"
    data-testid="proof-image-error"
  />
  <section
    v-else
    ref="scroller"
    class="h-full overflow-auto bg-bg"
    :aria-label="t('studio.proof.image.region.aria', { description })"
    data-testid="proof-image"
  >
    <img
      :src="`data:image/jpeg;base64,${payload.base64}`"
      :alt="description"
      :width="payload.width"
      :height="payload.height"
      class="mx-auto block h-auto max-w-full"
      decoding="async"
      @load="onLoad"
      @error="onError"
    >
  </section>
</template>
