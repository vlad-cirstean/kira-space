<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
</script>

<template>
  <!-- 72px label column: one fixed width so every item's values line up across the dialog (design seam). -->
  <KeyValueList
    class="grid-cols-[72px_minmax(0,1fr)] items-baseline [&>dt]:text-kira-sm [&>dt]:text-subtle"
    data-testid="approval-details"
  >
    <slot />
  </KeyValueList>
</template>
