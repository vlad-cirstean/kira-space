<script setup lang="ts">
import { siKubernetes } from 'simple-icons';

defineProps<{ size: number }>();
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" aria-hidden="true" class="shrink-0">
    <path :fill="`#${siKubernetes.hex}`" :d="siKubernetes.path" />
  </svg>
</template>
