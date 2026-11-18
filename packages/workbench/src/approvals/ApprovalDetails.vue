<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
</script>

<template>
  <!-- 72px label column: one fixed width so every item's values line up across the dialog (design seam). -->
  <KeyValueList columns="fixed" density="compact" data-testid="approval-details">
    <slot />
  </KeyValueList>
</template>
