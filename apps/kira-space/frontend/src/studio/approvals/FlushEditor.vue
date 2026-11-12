<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import { computed, ref } from 'vue';
import MonacoHost from '../editor/MonacoHost.vue';
import type { SqlDialect } from '../views/shared/sqlIdent';
import { useCodeLineHeight } from './codeLine';

// A read-only, wrapped, unboxed editor that is as tall as its text; the dialog body scrolls.
defineProps<{ doc: string; language: EditorLanguageId; sqlDialect?: SqlDialect }>();

const lineHeight = useCodeLineHeight();
// Null until Monaco lays out: the host's pending preview then shows at its natural height.
const contentHeight = ref<number | null>(null);
const height = computed(() =>
  contentHeight.value === null ? undefined : `${Math.max(contentHeight.value, lineHeight.value)}px`,
);
</script>

<template>
  <div class="overflow-hidden" :style="{ height }">
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
