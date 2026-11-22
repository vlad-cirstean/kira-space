<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import CodeBlock from '@theme/components/CodeBlock.vue';
import OpLogCodeBlock from '../components/OpLogCodeBlock.vue';

// With `text` the block is colourized by `language` (one highlighter for SQL, JSON and YAML); without
// it the slot is shown plain.
defineProps<{ text?: string; language?: EditorLanguageId | null }>();
</script>

<template>
  <!-- Approval content is unboxed: no fill and padding, data font at content size. -->
  <OpLogCodeBlock v-if="text !== undefined" unboxed :text="text" :language="language" />
  <CodeBlock v-else class="bg-transparent p-0 text-kira-md leading-normal"><slot /></CodeBlock>
</template>
