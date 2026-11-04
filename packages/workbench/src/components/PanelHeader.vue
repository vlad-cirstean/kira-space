<script setup lang="ts">
// The panel header bar every workbench panel shares: optional leading content (view tabs), title, then right-aligned actions.
</script>

<template>
  <div
    class="flex items-center shrink-0 h-bar gap-1 px-1.5 text-kira-sm text-muted-foreground uppercase tracking-wider"
    data-slot="panel-header"
  >
    <div v-if="$slots.start" class="flex items-center gap-1 normal-case tracking-normal"
      ><slot name="start" /></div
    >
    <span v-if="$slots.default" class="font-medium"><slot /></span>
    <div class="ml-auto flex items-center gap-1"><slot name="actions" /></div>
  </div>
</template>
