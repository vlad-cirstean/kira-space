<script setup lang="ts">
import { SHORTCUTS } from '@shared/domain/shortcuts';
import DialogOverlay from '@theme/components/ui/dialog/DialogOverlay.vue';
import DialogResizeHandles from '@theme/components/ui/dialog/DialogResizeHandles.vue';
import {
  CONFIRM_DISCARD_KEY,
  DIALOG_PROMPT_KEY,
  DIALOG_REQUEST_CLOSE_KEY,
} from '@theme/components/ui/dialog/discard';
import type { DialogSize } from '@theme/components/ui/dialog/resizeSize';
import { t } from '@theme/i18n';
import { dialogSurface } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { reactiveOmit, useEventListener, useTimeoutFn } from '@vueuse/core';
import type { DialogContentEmits, DialogContentProps } from 'reka-ui';
import {
  DialogContent,
  DialogPortal,
  injectDialogRootContext,
  useForwardExpose,
  useForwardPropsEmits,
} from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { computed, inject, provide, ref, watch } from 'vue';
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
      /** Prompt dialog: no initial focus on an action, single keys inert, Shift+Esc hides. */
      prompt?: boolean;
    }
  >(),
  {
    fixedHeight: false,
    destructive: false,
    dirty: false,
    resizable: false,
    prompt: false,
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
  'prompt',
);

const forwarded = useForwardPropsEmits(delegatedProps, emits);

// Initial focus: a destructive dialog opens on its header X (`data-dialog-dismiss`) so Enter
// closes instead of confirming; any other dialog may name a control with `data-dialog-initial`.
function onOpenAutoFocus(event: Event): void {
  const root = event.currentTarget as HTMLElement | null;
  const target = root?.querySelector<HTMLElement>(
    props.destructive ? '[data-dialog-dismiss]' : '[data-dialog-initial]',
  );
  if (!target) {
    // A prompt opens on its inert root, so a key typed elsewhere cannot press a button.
    if (props.prompt && root) {
      event.preventDefault();
      root.focus();
    }
    return;
  }
  event.preventDefault();
  target.focus();
}

const rootContext = injectDialogRootContext();
const confirmDiscard = inject(CONFIRM_DISCARD_KEY, null);
let asking = false;

// The X, Esc and the prompt hide chord close through here; the X is the only dismiss button.
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
provide(DIALOG_PROMPT_KEY, props.prompt);

function onEscapeKeyDown(event: KeyboardEvent): void {
  if (props.prompt) {
    event.preventDefault();
    return;
  }
  if (!props.dirty) return;
  event.preventDefault();
  void requestClose();
}

const HIDE_CHORD = SHORTCUTS['prompt.hide'].chord;
function onPromptKeydown(event: KeyboardEvent): void {
  if (!props.prompt || event.isComposing || event.key !== HIDE_CHORD.key) return;
  if (event.shiftKey !== Boolean(HIDE_CHORD.shift)) return;
  if (event.ctrlKey || event.metaKey || event.altKey) return;
  event.preventDefault();
  void requestClose();
}

// A prompt can open while the user is mid-chord in another surface: Mod+Enter runs a query and
// Mod+Backspace deletes rows there, and both answer a prompt here. Until the prompt has been open
// PROMPT_CHORD_GRACE_MS and a Mod keydown started after it opened, those chords are swallowed.
const PROMPT_CHORD_GRACE_MS = 700;
const chordsArmed = ref(!props.prompt);
let modSeen = false;
const { start: armLater, stop: cancelArm } = useTimeoutFn(
  () => {
    chordsArmed.value = true;
  },
  PROMPT_CHORD_GRACE_MS,
  { immediate: false },
);
watch(
  () => props.prompt && rootContext.open.value,
  (open) => {
    cancelArm();
    chordsArmed.value = !props.prompt;
    modSeen = false;
    if (open) armLater();
  },
  { immediate: true },
);
useEventListener(
  window,
  'keydown',
  (e: KeyboardEvent) => {
    if (!props.prompt || !rootContext.open.value) return;
    if (e.key === 'Meta' || e.key === 'Control') {
      if (!e.repeat) modSeen = true;
      return;
    }
    if (!(e.metaKey || e.ctrlKey) || (e.key !== 'Enter' && e.key !== 'Backspace')) return;
    if (chordsArmed.value && modSeen && !e.repeat) return;
    e.preventDefault();
    e.stopImmediatePropagation();
  },
  { capture: true },
);

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
    dialogSurface,
    'data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-95 data-open:zoom-in-95 text-kira-md duration-100 fixed top-1/2 left-1/2 z-(--kira-z-dialog) -translate-x-1/2 -translate-y-1/2 select-text outline-none',
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
      :data-chords-armed="prompt ? chordsArmed : undefined"
      :tabindex="prompt ? -1 : undefined"
      @keydown="onPromptKeydown"
      @open-auto-focus="onOpenAutoFocus"
      @escape-key-down="onEscapeKeyDown"
      @pointer-down-outside="onPointerDownOutside"
    >
      <slot />
      <DialogResizeHandles v-if="resizable" v-model="size" :target="currentElement" />
    </DialogContent>
  </DialogPortal>
</template>
