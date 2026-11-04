<script setup lang="ts">
import DialogOverlay from '@theme/components/ui/dialog/DialogOverlay.vue';
import DialogResizeHandles from '@theme/components/ui/dialog/DialogResizeHandles.vue';
import { CONFIRM_DISCARD_KEY, DIALOG_REQUEST_CLOSE_KEY } from '@theme/components/ui/dialog/discard';
import type { DialogSize } from '@theme/components/ui/dialog/resizeSize';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { DialogContentEmits, DialogContentProps } from 'reka-ui';
import {
  DialogContent,
  DialogPortal,
  injectDialogRootContext,
  useForwardExpose,
  useForwardPropsEmits,
} from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { computed, inject, provide, ref } from 'vue';
import type { DialogContentSizeVariants } from '.';
import { dialogContentSizeVariants } from '.';

defineOptions({
  inheritAttrs: false,
});

// P262: `size` is required. Header, body and footer own their padding; the close button lives in
// DialogHeader (`closable`).
const props = withDefaults(
  defineProps<
    DialogContentProps & {
      class?: HTMLAttributes['class'];
      size: NonNullable<DialogContentSizeVariants['size']>;
      fixedHeight?: boolean;
      destructive?: boolean;
      /** Unsaved edits: an outside click is ignored; Esc and the header X ask to discard first. */
      dirty?: boolean;
      /** Right and bottom edge handles; the dialog grows from its centre and never exceeds the size variant's bounds. */
      resizable?: boolean;
    }
  >(),
  {
    fixedHeight: false,
    destructive: false,
    dirty: false,
    resizable: false,
  },
);
const emits = defineEmits<DialogContentEmits>();

const delegatedProps = reactiveOmit(
  props,
  'class',
  'size',
  'fixedHeight',
  'destructive',
  'dirty',
  'resizable',
);

const forwarded = useForwardPropsEmits(delegatedProps, emits);

// Initial focus: a destructive dialog opens on its dismiss control (`data-dialog-dismiss`) so Enter
// cancels instead of confirming; any other dialog may name a control with `data-dialog-initial`.
function onOpenAutoFocus(event: Event): void {
  const root = event.currentTarget as HTMLElement | null;
  const target = root?.querySelector<HTMLElement>(
    props.destructive ? '[data-dialog-dismiss]' : '[data-dialog-initial]',
  );
  if (!target) return;
  event.preventDefault();
  target.focus();
}

const rootContext = injectDialogRootContext();
const confirmDiscard = inject(CONFIRM_DISCARD_KEY, null);
let asking = false;

// Footer Cancel never asks: it is a plain DialogClose. Only the X and Esc route through here.
async function requestClose(): Promise<void> {
  if (asking) return;
  if (props.dirty && confirmDiscard) {
    asking = true;
    try {
      const discard = await confirmDiscard({
        title: t('common.discard.title'),
        body: t('common.discard.body'),
        confirmLabel: t('common.action.discard.label'),
      });
      if (!discard) return;
    } finally {
      asking = false;
    }
  }
  rootContext.onOpenChange(false);
}
provide(DIALOG_REQUEST_CLOSE_KEY, requestClose);

function onEscapeKeyDown(event: KeyboardEvent): void {
  if (!props.dirty) return;
  event.preventDefault();
  void requestClose();
}

function onPointerDownOutside(event: Event): void {
  if (props.dirty) event.preventDefault();
}

const { forwardRef, currentElement } = useForwardExpose();
const size = ref<DialogSize>({ width: null, height: null });
const sizeStyle = computed(() =>
  props.resizable
    ? {
        width: size.value.width === null ? undefined : `${size.value.width}px`,
        height: size.value.height === null ? undefined : `${size.value.height}px`,
      }
    : undefined,
);

const contentClass = computed(() =>
  cn(
    'bg-elevated text-fg data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-95 data-open:zoom-in-95 ring-border-strong rounded-kira-lg text-kira-md ring-1 duration-100 fixed top-1/2 left-1/2 z-(--kira-z-dialog) shadow-kira-dialog -translate-x-1/2 -translate-y-1/2 outline-none',
    dialogContentSizeVariants({ size: props.size, fixedHeight: props.fixedHeight }),
    props.class,
  ),
);
</script>

<template>
  <DialogPortal>
    <DialogOverlay />
    <DialogContent
      data-slot="dialog-content"
      v-bind="{ ...$attrs, ...forwarded }"
      :ref="forwardRef"
      :class="contentClass"
      :style="sizeStyle"
      :data-dirty="dirty || undefined"
      @open-auto-focus="onOpenAutoFocus"
      @escape-key-down="onEscapeKeyDown"
      @pointer-down-outside="onPointerDownOutside"
    >
      <slot />
      <DialogResizeHandles v-if="resizable" v-model="size" :target="currentElement" />
    </DialogContent>
  </DialogPortal>
</template>
