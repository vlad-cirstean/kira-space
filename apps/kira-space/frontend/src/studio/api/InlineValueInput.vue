<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { useFocusOnMount } from '@workbench/behaviour/focus';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import { type ComponentPublicInstance, useTemplateRef } from 'vue';

// A cell or row field that edits in place: focused on mount, Enter and blur commit, Esc cancels. The
// parent swaps it out on `commit` and `cancel`. A blank commit is a cancel unless `allowEmpty`
// (clearing a value, a description). The text is trimmed.
const props = defineProps<{
  value: string;
  label: string;
  type?: 'text' | 'password';
  placeholder?: string;
  allowEmpty?: boolean;
}>();
const emit = defineEmits<{ commit: [text: string]; cancel: [] }>();

const input = useTemplateRef<ComponentPublicInstance>('input');
useFocusOnMount(input, { select: true });

const edit = useInlineEdit({
  source: () => props.value,
  commit: (text) => emit('commit', text),
  cancel: () => emit('cancel'),
  allowEmpty: props.allowEmpty,
  once: true,
});
</script>

<template>
  <Input
    ref="input"
    :model-value="edit.text.value"
    :type="type ?? 'text'"
    size="kira"
    class="font-data"
    :aria-label="label"
    :placeholder="placeholder"
    autocomplete="off"
    @input="edit.onInput(($event.target as HTMLInputElement).value)"
    @click.stop
    @keydown="edit.onKeydown"
    @blur="edit.onCommit"
  />
</template>
