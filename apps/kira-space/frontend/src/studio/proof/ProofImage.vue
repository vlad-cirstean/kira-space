<script setup lang="ts">
import type { ProofImagePayload } from '@shared/domain/proof';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';

// One image step (PNG, JPEG, WebP or SVG): the file's bytes as a data URL (the window CSP allows
// data:, not blob:), fitted whole into the pane (width and height), never upscaled, so one view and
// each screenshot show all of it. SVG renders only through <img>, never inline, so its scripts
// cannot run.
const props = defineProps<{ payload: ProofImagePayload; description: string }>();
const emit = defineEmits<{ ready: [] }>();

const MIME = {
  png: 'image/png',
  jpeg: 'image/jpeg',
  webp: 'image/webp',
  svg: 'image/svg+xml',
} as const;
const src = computed(() => `data:${MIME[props.payload.format]};base64,${props.payload.base64}`);
const failed = ref(false);

let announced = false;
function announce(): void {
  if (announced) return;
  announced = true;
  emit('ready');
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
    class="absolute inset-0 flex items-center justify-center overflow-hidden bg-bg"
    :aria-label="t('studio.proof.image.region.aria', { description })"
    data-testid="proof-image"
  >
    <img
      :src="src"
      :alt="description"
      :width="payload.width"
      :height="payload.height"
      class="block h-auto max-h-full min-h-0 w-auto max-w-full min-w-0"
      decoding="async"
      @load="announce"
      @error="onError"
    >
  </section>
</template>
