<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import { computed, ref } from 'vue';
import MonacoHost from '../editor/MonacoHost.vue';
import type { SqlDialect } from '../views/shared/sqlIdent';
import { useCodeLineHeight } from './codeLine';

// A read-only, wrapped, unboxed editor that is as tall as its text, up to 12 lines.
defineProps<{ doc: string; language: EditorLanguageId; sqlDialect?: SqlDialect }>();

const MAX_LINES = 12;
const lineHeight = useCodeLineHeight();
const contentHeight = ref(0);
const height = computed(() =>
  Math.min(Math.max(contentHeight.value, lineHeight.value), MAX_LINES * lineHeight.value),
);
</script>

<template>
  <div class="overflow-hidden" :style="{ height: `${height}px` }">
    <MonacoHost
      :doc="doc"
      :language="language"
      :sql-dialect="sqlDialect"
      readonly
      flush
      force-wrap
      @update:content-height="(h) => (contentHeight = h)"
    />
  </div>
</template>
