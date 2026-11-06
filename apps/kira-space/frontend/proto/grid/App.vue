<script setup lang="ts">
import { TooltipProvider } from '@theme/components/ui/tooltip';
import ContextMenu from '@workbench/components/ContextMenu.vue';
import { defineAsyncComponent } from 'vue';
import CheetahProto from './CheetahProto.vue';

const Hud = __KIRA_DEBUG_HOOKS__ ? defineAsyncComponent(() => import('./Hud.vue')) : null;
</script>

<template>
  <TooltipProvider>
    <div class="flex h-screen w-screen flex-col bg-bg text-fg">
      <CheetahProto />
    </div>
    <ContextMenu />
    <component :is="Hud" v-if="Hud" />
  </TooltipProvider>
</template>
