<script setup lang="ts">
import { MASKED_TEXT } from '@theme/format';

// A hidden secret value: the one mask text, subtle and widely tracked. Attrs fall through.
</script>

<template>
  <span data-slot="masked-value" class="text-subtle tracking-widest">{{ MASKED_TEXT }}</span>
</template>
