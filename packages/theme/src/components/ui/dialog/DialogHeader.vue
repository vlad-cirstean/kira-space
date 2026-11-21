<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Button } from '@theme/components/ui/button';
import DialogClose from '@theme/components/ui/dialog/DialogClose.vue';
import { DIALOG_PROMPT_KEY, DIALOG_REQUEST_CLOSE_KEY } from '@theme/components/ui/dialog/discard';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';
import { inject } from 'vue';
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
</script>

<template>
  <div data-slot="dialog-header" :class="cn(dialogHeaderClass, props.class)">
    <div class="flex min-w-0 flex-1 flex-col gap-0.5"><slot /></div>
    <template v-if="closable">
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
    </template>
  </div>
</template>
