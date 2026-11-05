<script setup lang="ts">
import Command from '@theme/components/ui/command/Command.vue';
import {
  Dialog,
  DialogDescription,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import { floatingSurface } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import type { DialogRootEmits, DialogRootProps } from 'reka-ui';
import { DialogContent, DialogPortal, useForwardPropsEmits } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

// P262 §2.2 exempt: the command surface keeps its own top-anchored, header-less box, so it renders
// reka's content directly instead of the sized DialogContent.
const props = defineProps<
  DialogRootProps & {
    title?: string;
    description?: string;
    class?: HTMLAttributes['class'];
  }
>();
const emits = defineEmits<DialogRootEmits>();

const forwarded = useForwardPropsEmits(props, emits);
</script>

<template>
  <Dialog v-slot="slotProps" v-bind="forwarded">
    <DialogPortal>
      <DialogOverlay />
      <DialogContent
        data-slot="dialog-content"
        :class="
          cn(
            floatingSurface,
            'data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-95 data-open:zoom-in-95 fixed left-1/2 z-(--kira-z-dialog) grid w-full max-w-[calc(100%-2rem)] -translate-x-1/2 gap-4 overflow-hidden p-0 text-kira-md duration-100 outline-none top-1/3 translate-y-0',
            props.class,
          )
        "
      >
        <DialogHeader class="sr-only" :closable="false">
          <DialogTitle>{{ title ?? t('theme.commandDialog.title') }}</DialogTitle>
          <DialogDescription>{{
            description ?? t('theme.commandDialog.description')
          }}</DialogDescription>
        </DialogHeader>
        <Command>
          <slot v-bind="slotProps" />
        </Command>
      </DialogContent>
    </DialogPortal>
  </Dialog>
</template>
