<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { useFocusOnMount } from '@workbench/behaviour/focus';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import { type ComponentPublicInstance, useTemplateRef } from 'vue';

// One status name field: Enter and blur save, Esc reverts. `commit` rejects to keep the text and flag it.
const props = defineProps<{
  name: string;
  label: string;
  focus?: boolean;
  commit: (name: string) => Promise<void>;
}>();

const input = useTemplateRef<ComponentPublicInstance>('input');
if (props.focus) useFocusOnMount(input, { select: true });

const edit = useInlineEdit({ source: () => props.name, commit: (text) => props.commit(text) });
</script>

<template>
  <Input
    ref="input"
    :model-value="edit.text.value"
    :aria-label="label"
    :aria-invalid="edit.error.value ? true : undefined"
    :title="edit.error.value || undefined"
    @update:model-value="edit.onInput"
    @keydown="edit.onKeydown"
    @blur="edit.onCommit"
  />
</template>
