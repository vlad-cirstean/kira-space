<script setup lang="ts">
import LogLines, { type LogLine } from '@workbench/components/LogLines.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import { useLog } from '../readQueries';
import type { LogChunk, LogKind } from '../wire';

// Inline log of one run or worktree setup (R28): the first page, then pushed chunks, following the
// tail unless the reader scrolled up.
const props = defineProps<{ kind: LogKind; id: string; maxHeight?: string }>();

const log = useLog(
  () => props.kind,
  () => props.id,
);

// One line object per chunk: a push appends chunks and keeps the old ones, so the exit-code regex
// runs once per chunk and the list keeps the identity of every line it already rendered.
const lineOf = new WeakMap<LogChunk, LogLine>();
function toLine(c: LogChunk): LogLine {
  let line = lineOf.get(c);
  if (!line) {
    line = {
      id: c.seq,
      text: c.text,
      stream: c.stream === 'stderr' || /exit code [1-9]/i.test(c.text) ? 'stderr' : c.stream,
    };
    lineOf.set(c, line);
  }
  return line;
}
const lines = computed(() => (log.data.value?.chunks ?? []).map(toLine));
</script>

<template>
  <QueryState :query="log" variant="inline" :is-empty="() => false">
    <template #error="{ message }">
      <LogLines
        :lines="[]"
        :error="message"
        testid="ade-run-log"
        :max-height="maxHeight"
        :data-log-id="id"
      />
    </template>
    <template #default="{ data }">
      <LogLines
        :lines="lines"
        :truncated="data.truncated"
        testid="ade-run-log"
        :max-height="maxHeight"
        :data-log-id="id"
      />
    </template>
  </QueryState>
</template>
