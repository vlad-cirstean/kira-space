<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Button } from '@theme/components/ui/button';

// `click` emits the button element itself so a caller anchoring a dropdown menu
// (contextMenu's own openContextMenuAt) has a rect to anchor against without a second ref.
defineProps<{
  label: string;
  tooltip: string;
  hasPopup?: boolean;
}>();

const emit = defineEmits<{ click: [button: HTMLButtonElement] }>();

function onClick(e: MouseEvent): void {
  emit('click', e.currentTarget as HTMLButtonElement);
}
</script>

<template>
  <div class="h-full flex items-center shrink-0 pr-1 pl-0.5" data-testid="tab-strip-actions">
    <TextTooltip :text="tooltip">
      <Button
        variant="toolbar"
        size="title"
        :aria-label="label"
        :aria-haspopup="hasPopup ? 'menu' : undefined"
        data-testid="tab-strip-new"
        @click="onClick"
      >
        <CodiconIcon name="add" :size="13" />
      </Button>
    </TextTooltip>
  </div>
</template>
