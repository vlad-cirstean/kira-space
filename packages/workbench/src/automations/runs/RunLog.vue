<script setup lang="ts">
import LogLines from '@workbench/components/LogLines.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { useRunLog } from './runsQueries';

// A smart run's log: the first page, then pushed chunks, following the end unless the reader
// scrolled up.
const props = defineProps<{ runId: string }>();

const log = useRunLog(() => props.runId);
</script>

<template>
  <div class="flex min-h-48 flex-1 flex-col">
    <QueryState :query="log" variant="inline" :is-empty="() => false">
      <template #error="{ message }">
        <LogLines testid="run-log" max-height="none" :lines="[]" :error="message" />
      </template>
      <template #default="{ data }">
        <LogLines
          testid="run-log"
          max-height="none"
          :truncated="data.truncated"
          :lines="data.chunks.map((c) => ({ id: c.seq, text: c.text, stream: c.stream }))"
        />
      </template>
    </QueryState>
  </div>
</template>
