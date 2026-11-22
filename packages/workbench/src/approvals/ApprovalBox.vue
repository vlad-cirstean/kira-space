<script setup lang="ts">
// One framed section of an approval card. The card frames a whole body in one; a kind with
// `sections` frames each of its sections in its own.
withDefaults(defineProps<{ testid?: string }>(), { testid: undefined });
</script>

<template>
  <section
    class="flex min-w-0 flex-col gap-2 rounded-kira-sm border border-border p-3"
    data-approval-box
    :data-testid="testid"
  >
    <slot />
  </section>
</template>
