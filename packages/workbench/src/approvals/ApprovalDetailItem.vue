<script setup lang="ts">
// One Details row: label cell and value cell, so no Details file writes the pair by hand.
defineProps<{ label: string; testid?: string }>();
</script>

<template>
  <dt>{{ label }}</dt>
  <dd :data-testid="testid"><slot /></dd>
</template>
