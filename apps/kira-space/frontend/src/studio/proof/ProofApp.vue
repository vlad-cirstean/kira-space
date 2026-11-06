<script setup lang="ts">
import { TooltipProvider } from '@theme/components/ui/tooltip';
import AppLiveRegion from '@workbench/components/AppLiveRegion.vue';
import ConfirmDialog from '@workbench/components/ConfirmDialog.vue';
import ContextMenu from '@workbench/components/ContextMenu.vue';
import ProofWindow from './ProofWindow.vue';
</script>

<template>
  <TooltipProvider>
    <ProofWindow />
    <ConfirmDialog />
    <AppLiveRegion />
    <ContextMenu />
  </TooltipProvider>
</template>
