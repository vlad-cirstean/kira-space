<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { proofReviewPayloadSchema } from '@shared/domain/proof';
import { computed } from 'vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => proofReviewPayloadSchema.parse(props.item.payload));

function diffLines(diff: string): string[] {
  return diff.replace(/\n$/, '').split('\n');
}

// Full literals: Tailwind emits only scanned classes.
function lineClass(line: string): string {
  if (line.startsWith('-')) return 'text-status-red';
  if (line.startsWith('+')) return 'text-status-green';
  return '';
}
</script>

<template>
  <ol class="m-0 flex list-none flex-col gap-2 p-0" data-testid="proof-review-body">
    <li
      v-for="step in payload.steps"
      :key="step.ordinal"
      class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-2"
    >
      <span class="text-subtle tabular-nums">{{ step.ordinal }}</span>
      <span class="min-w-0">{{ step.description }}</span>
      <div
        v-if="step.diff"
        class="col-start-2 max-h-48 overflow-auto whitespace-pre font-data text-kira-md leading-normal"
      >
        <span
          v-for="(line, i) in diffLines(step.diff)"
          :key="i"
          class="block"
          :class="lineClass(line)"
          >{{
            line || '\u00a0'
          }}</span
        >
      </div>
    </li>
  </ol>
</template>
