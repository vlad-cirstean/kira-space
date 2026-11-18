<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { type ComponentPublicInstance, useTemplateRef } from 'vue';
import { useFocusOnMount } from '../behaviour/focus';
import { useInlineEdit } from '../behaviour/inlineEdit';

// The inline row-name field (API collection tree, Automations panel): Enter and blur commit, Esc
// cancels, a blank or unchanged name is a cancel (the row would become unclickable). The parent
// swaps the field out on `commit`/`cancel`.
const props = withDefaults(
  defineProps<{ name: string; label: string; selectOnFocus?: boolean }>(),
  {
    selectOnFocus: true,
  },
);
const emit = defineEmits<{ commit: [name: string]; cancel: [] }>();

const input = useTemplateRef<ComponentPublicInstance>('input');
useFocusOnMount(input, { select: props.selectOnFocus });

const edit = useInlineEdit({
  source: () => props.name,
  commit: (text) => emit('commit', text),
  cancel: () => emit('cancel'),
  once: true,
});
</script>

<template>
  <Input
    ref="input"
    :model-value="edit.text.value"
    appearance="inline"
    size="kira"
    class="flex-1"
    :aria-label="label"
    :aria-invalid="edit.error.value ? true : undefined"
    :title="edit.error.value || undefined"
    @input="edit.onInput(($event.target as HTMLInputElement).value)"
    @click.stop
    @dblclick.stop
    @keydown="edit.onKeydown"
    @blur="edit.onCommit"
  />
</template>
