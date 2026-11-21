<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Button } from '@theme/components/ui/button';
import DialogClose from '@theme/components/ui/dialog/DialogClose.vue';
import { DIALOG_PROMPT_KEY, DIALOG_REQUEST_CLOSE_KEY } from '@theme/components/ui/dialog/discard';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { useEventListener } from '@vueuse/core';
import type { HTMLAttributes } from 'vue';
import { inject, useTemplateRef } from 'vue';
import { dialogHeaderClass } from '.';

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'];
    closable?: boolean;
    closeTestid?: string;
  }>(),
  {
    closable: true,
  },
);

// Inside a `DialogContent` the X closes through its dirty guard; elsewhere it is a plain close.
const requestClose = inject(DIALOG_REQUEST_CLOSE_KEY, null);
// Esc closes a dialog, Shift+Esc a prompt; the X runs the same close.
const isPrompt = inject(DIALOG_PROMPT_KEY, false);
// Dialog focus lands on the X; a focus-opened tooltip would take the first Esc meant for the dialog.
// Only that programmatic focus is swallowed: Tab focus (the same Tab's focus move) shows the chord.
// The flag clears right after the Tab's default action, so a later pointer or async open never sees it.
const closeEl = useTemplateRef<HTMLElement>('closeEl');
let tabbing = false;
useEventListener(
  document,
  'keydown',
  (e) => {
    if (e.key !== 'Tab') return;
    tabbing = true;
    setTimeout(() => {
      tabbing = false;
    });
  },
  { capture: true },
);
useEventListener(
  closeEl,
  'focus',
  (e) => {
    if (!tabbing) e.stopPropagation();
  },
  { capture: true },
);
</script>

<template>
  <div data-slot="dialog-header" :class="cn(dialogHeaderClass, props.class)">
    <div class="flex min-w-0 flex-1 flex-col gap-0.5"><slot /></div>
    <span v-if="closable" ref="closeEl" class="contents">
      <TextTooltip
        v-if="requestClose"
        :text="t('common.action.close.label')"
        :shortcut="isPrompt ? 'prompt.hide' : 'form.cancel'"
      >
        <Button
          variant="ghost"
          size="icon-sm"
          class="ml-auto pointer-coarse:size-touch"
          :aria-label="t('common.action.close.label')"
          data-dialog-dismiss
          :data-testid="closeTestid"
          @click="requestClose"
        >
          <CodiconIcon name="close" :size="13" />
        </Button>
      </TextTooltip>
      <TextTooltip v-else :text="t('common.action.close.label')" shortcut="form.cancel">
        <DialogClose as-child>
          <Button
            variant="ghost"
            size="icon-sm"
            class="ml-auto pointer-coarse:size-touch"
            :aria-label="t('common.action.close.label')"
            data-dialog-dismiss
            :data-testid="closeTestid"
          >
            <CodiconIcon name="close" :size="13" />
          </Button>
        </DialogClose>
      </TextTooltip>
    </span>
  </div>
</template>
