<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import { floatingSurface } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import type { ToasterProps } from 'vue-sonner';
import { Toaster } from 'vue-sonner';
import 'vue-sonner/style.css';

const props = defineProps<Pick<ToasterProps, 'position'>>();

// Unstyled: the theme's floating surface replaces vue-sonner's own look.
const toastOptions = {
  unstyled: true,
  classes: {
    toast: cn(
      floatingSurface,
      'flex w-(--width) items-center gap-2 p-2.5 text-kira-md pointer-events-auto',
    ),
    title: 'text-kira-md font-medium text-fg',
    description: 'text-kira-sm text-muted-foreground',
  },
};
</script>

<template>
  <Toaster
    :position="props.position"
    :toast-options="toastOptions"
    class="z-(--kira-z-popover)"
    data-slot="sonner"
  >
    <template #loading-icon><Spinner label="" /></template>
    <template #success-icon><CodiconIcon name="check" :size="13" class="text-ok" /></template>
    <template #error-icon><CodiconIcon name="error" :size="13" class="text-error" /></template>
    <template #warning-icon><CodiconIcon name="warning" :size="13" class="text-warn" /></template>
    <template #info-icon
      ><CodiconIcon name="info" :size="13" class="text-muted-foreground" /></template
    >
  </Toaster>
</template>
