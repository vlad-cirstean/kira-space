<script setup lang="ts">
import PrGlyph from '@ade/pr/PrGlyph.vue';
import { useChangeRequests } from '@ade/readQueries';
import { computed } from 'vue';

// The pull requests of a task's branches, one glyph with its status lines each.
const props = defineProps<{ branchIds: readonly string[] }>();

const crs = useChangeRequests();
const items = computed(() =>
  props.branchIds.flatMap((id) => {
    const pr = crs.data.value?.branches[id];
    return pr ? [{ id, pr }] : [];
  }),
);
</script>

<template>
  <span v-if="items.length" class="flex flex-col gap-1.5">
    <span
      v-for="{ id, pr } in items"
      :key="id"
      class="flex flex-col gap-0.5"
      :data-branch-id="id"
      data-testid="plan-task-pr"
    >
      <span class="truncate text-kira-sm text-fg">#{{ pr.number }} {{ pr.title }}</span>
      <PrGlyph variant="inline" :status="pr" :base-ref="pr.baseRef" />
    </span>
  </span>
</template>
