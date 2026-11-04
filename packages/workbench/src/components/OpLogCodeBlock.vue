<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import { cn } from '@theme/lib/utils';
import { computed, ref, watch } from 'vue';
import { loadMonaco } from '../editor/monaco';
import { monacoLanguageIdFor } from '../editor/monacoLanguages';
import { paintOverlayHtml } from '../editor/paintSpans';

// A read-only, wrapped, scrollable text block. With a `language` it is syntax-coloured through
// Monaco's `colorize` (no editor instance); plain text until that resolves.
const props = defineProps<{ text: string; language?: EditorLanguageId | null; tone?: 'error' }>();

const html = ref<string | null>(null);
let generation = 0;
watch(
  () => [props.text, props.language] as const,
  async ([text, language]) => {
    const mine = ++generation;
    if (!language) {
      html.value = null;
      return;
    }
    try {
      const mod = await loadMonaco();
      const id = monacoLanguageIdFor(language);
      const lines = await Promise.all(
        text.split('\n').map((line) => paintOverlayHtml(mod, line, id, [])),
      );
      if (mine === generation) html.value = lines.join('\n');
    } catch {
      if (mine === generation) html.value = null;
    }
  },
  { immediate: true },
);

const klass = computed(() =>
  cn(
    'm-0 max-h-[calc(12*1lh)] overflow-auto rounded-kira-sm bg-bg px-1.5 py-1 font-data break-all whitespace-pre-wrap select-text',
    props.tone === 'error' && 'text-error',
  ),
);
</script>

<template>
  <!-- v-html is paintOverlayHtml output only: escaped text in static class names. -->
  <pre v-if="html !== null" :class="klass" v-html="html"></pre>
  <pre v-else :class="klass">{{ text }}</pre>
</template>
