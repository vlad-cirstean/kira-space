<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { ICON } from '@theme/icons';
import { useCopy } from '../behaviour/clipboard';

// The one icon-only copy button: `copy` glyph, `check` for `TIMING.copiedFeedback` after a copy,
// error tone after a rejected write. `text` may be a getter for a value built on click. A copy
// click never means the surrounding row's action, so it stops propagation. Attrs (testid, size,
// class) fall through to the button.
const props = defineProps<{
  text: string | (() => string | Promise<string>);
  label: string;
  /** Announced on success instead of the generic "Copied". */
  message?: string;
  disabled?: boolean;
}>();

const { copy, copied, error } = useCopy();

async function onClick(): Promise<void> {
  const text = typeof props.text === 'function' ? await props.text() : props.text;
  await copy(text, { message: props.message });
}
</script>

<template>
  <TooltipIconButton
    :icon="copied ? ICON.copied : ICON.copy"
    :label="label"
    :disabled="disabled"
    :tone="error ? 'error' : undefined"
    @click.stop="onClick"
  />
</template>
