<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
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
      <PopoverContent
        align="start"
        class="w-96 max-h-60 gap-0 overflow-hidden p-0 shadow-kira"
        data-testid="error-popover"
      >
        <FieldError
          class="overflow-auto whitespace-pre-wrap break-words p-2 font-data text-kira-md"
          >{{
            props.message
          }}</FieldError
        >
        <FieldError v-if="copyError" class="px-2 pb-1" data-testid="error-popover-copy-error">
          {{ t('common.copy.failed.error', { error: copyError }) }}
        </FieldError>
        <!-- Footer is the same 28px band used everywhere a toolbar sits at the edge of a floating
             surface, with the border on top since this one closes the popover. -->
        <ViewToolbar>
          <Button variant="toolbar" size="kira" class="ml-auto" @click="onCopy">{{
            t('common.action.copy.label')
          }}</Button>
          <Button variant="toolbar" size="kira" @click="open = false">{{
            t('common.action.close.label')
          }}</Button>
        </ViewToolbar>
      </PopoverContent>
    </Popover>
  </span>
</template>
