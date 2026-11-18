<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import {
  Popover,
  PopoverContent,
  PopoverFooter,
  PopoverTrigger,
} from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { ref } from 'vue';

// A "click for full text" replacement for truncated inline error text (unreadable in a native
// title tooltip for multi-line/long messages, and unreachable on touch).
const props = defineProps<{ message: string }>();

const open = ref(false);
const copyError = ref<string | null>(null);

function onCopy(): void {
  copyError.value = null;
  void copyOrReportError(props.message, (m) => {
    copyError.value = m;
  });
}
</script>

<template>
  <span class="min-w-0 ml-auto shrink">
    <Popover v-model:open="open" @update:open="copyError = null">
      <PopoverTrigger as-child>
        <button
          type="button"
          class="flex items-center min-w-0 max-w-full bg-transparent border-none p-0 gap-1 text-error text-kira-md"
          data-testid="error-popover-trigger"
          :aria-label="t('studio.project.errorPopover.aria', { message: props.message })"
          @click.stop
        >
          <CodiconIcon name="error" :size="13" />
          <span class="truncate">{{ props.message }}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" flush class="w-96 max-h-60" data-testid="error-popover">
        <FieldError
          class="overflow-auto whitespace-pre-wrap break-words p-2 font-data text-kira-md"
          >{{
            props.message
          }}</FieldError
        >
        <FieldError v-if="copyError" class="px-2 pb-1" data-testid="error-popover-copy-error">
          {{ t('common.copy.failed.error', { error: copyError }) }}
        </FieldError>
        <PopoverFooter>
          <Button variant="toolbar" size="kira" @click="onCopy"
            ><CodiconIcon name="copy" :size="13" />{{ t('common.action.copy.label') }}</Button
          >
          <Button variant="toolbar" size="kira" @click="open = false"
            ><CodiconIcon name="close" :size="13" />{{ t('common.action.close.label') }}</Button
          >
        </PopoverFooter>
      </PopoverContent>
    </Popover>
  </span>
</template>
