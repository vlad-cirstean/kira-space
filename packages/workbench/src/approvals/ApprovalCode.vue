<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
</script>

<template>
  <!-- Approval content is unboxed: CodeBlock without its fill and padding, data font at content size. -->
  <CodeBlock class="bg-transparent p-0 text-kira-md leading-normal"><slot /></CodeBlock>
</template>
