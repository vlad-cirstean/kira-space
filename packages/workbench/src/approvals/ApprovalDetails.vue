<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
</script>

<template>
  <!-- One 6rem label column so every item's values line up across the dialog (design seam). -->
  <KeyValueList columns="approval" density="approval" data-testid="approval-details">
    <slot />
  </KeyValueList>
</template>
