<script setup lang="ts">
import { useFocusOnMount } from '@workbench/behaviour/focus';
import SecretInput from '@workbench/components/SecretInput.vue';
import { computed, ref, useTemplateRef } from 'vue';

// A secret cell's field: a SecretInput with its eye. Enter commits, Esc cancels, and focus leaving
// the field (not moving to its own eye) commits a typed value. A blank value is a cancel: the
// saved secret stays.
defineProps<{ set: boolean; label: string; placeholder: string }>();
const emit = defineEmits<{ commit: [value: string]; cancel: [] }>();

const value = ref('');
const secret = useTemplateRef<{ input: HTMLInputElement | undefined }>('secret');
useFocusOnMount(computed(() => secret.value?.input));
const box = useTemplateRef<HTMLElement>('box');

let settled = false;
function commit(): void {
  if (settled) return;
  settled = true;
  if (value.value === '') emit('cancel');
  else emit('commit', value.value);
}
function cancel(): void {
  if (settled) return;
  settled = true;
  emit('cancel');
}
function onFocusOut(e: FocusEvent): void {
  if (e.relatedTarget instanceof Node && box.value?.contains(e.relatedTarget)) return;
  commit();
}
</script>

<template>
  <!-- biome-ignore lint/a11y/noStaticElementInteractions: focusout and Enter/Escape bubble from the one input and its eye; the input is the focusable control. -->
  <div
    ref="box"
    class="min-w-0 flex-1"
    @click.stop
    @focusout="onFocusOut"
    @keydown.enter.prevent="commit"
    @keydown.esc.stop.prevent="cancel"
  >
    <SecretInput
      ref="secret"
      v-model="value"
      :set="set"
      :aria-label="label"
      :placeholder="placeholder"
      data-testid="matrix-cell-input"
    />
  </div>
</template>
