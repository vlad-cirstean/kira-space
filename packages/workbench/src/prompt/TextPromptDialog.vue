<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { dialogBodyClass, dialogFooterClass, dialogHeaderClass } from '@theme/components/ui/dialog';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { useEventListener } from '@vueuse/core';
import { FocusScope } from 'reka-ui';
import { type ComponentPublicInstance, onBeforeUnmount, ref, useId, useTemplateRef } from 'vue';
import { useFocusOnMount, useFocusReturn } from '../behaviour/focus';

// Deliberately NOT built on shadcn-vue's Dialog (declined, real requirement) — its
// DialogContent wraps reka-ui's DialogPortal, which teleports to document.body. Two of this
// composable's three original call sites (FilterHistoryMenu.vue, ConsoleSavedMenu.vue) raise this
// prompt from inside a reka-ui Popover's own #footer slot; both carry a test-caught fix
// (data-view.spec.ts/console.spec.ts) requiring the prompt stay a DOM descendant of that
// PopoverContent — a teleported node reads as an outside interaction to Popover's own dismiss
// layer and closes the menu before the prompt is seen. A plain, non-portaled `<div v-if>` (this
// file) is the one shape that works unchanged at both a popover-nested and a directly-raised
// (GitPanel.vue) call site.
defineProps<{ title: string }>();
const value = defineModel<string>({ required: true });
const emit = defineEmits<{ submit: []; close: [] }>();
const titleId = useId();

// ui/input's root IS the <input> element itself — no querySelector needed. onMounted fires each
// time this component is created, i.e. exactly when the caller's prompt state goes non-null.
const inputRef = ref<ComponentPublicInstance | null>(null);
const focusReturn = useFocusReturn();
focusReturn.capture();
useFocusOnMount(inputRef);
onBeforeUnmount(() => focusReturn.restore());

// The scrim is a pointer-event shield, not an interactive element — it absorbs a
// click so it never reaches the ancestor Popover's own outside-click dismiss layer (see the
// comment above). Not "click outside closes" (there is no close-on-backdrop behaviour here), so
// this is a plain listener move, not an onClickOutside swap.
const scrimEl = useTemplateRef<HTMLElement>('scrimEl');
useEventListener(scrimEl, 'click', (e) => e.stopPropagation());
</script>

<template>
  <!-- The dialog rung, not a bare low z-index — strictly higher, so unifying every call site
       on it (including GitPanel.vue, raised directly from the panel with no popover backdrop of its
       own to clear) is a safe superset, never a visible change at any of the three. -->
  <div
    ref="scrimEl"
    class="fixed inset-0 flex items-center justify-center bg-scrim z-(--kira-z-dialog)"
    data-testid="text-prompt"
  >
    <FocusScope as-child trapped loop>
      <div
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        class="mx-4 w-100 max-w-full flex flex-col overflow-hidden rounded-kira-lg bg-elevated text-fg text-kira-md border border-border"
      >
        <div :class="dialogHeaderClass">
          <div :id="titleId" class="text-kira-lg">{{ title }}</div>
        </div>
        <div :class="dialogBodyClass">
          <Input
            ref="inputRef"
            v-model="value"
            size="kira-lg"
            :aria-labelledby="titleId"
            data-testid="text-prompt-input"
            @keydown="wrapSelectionOnType"
            @keydown.enter="(e: KeyboardEvent) => !e.isComposing && emit('submit')"
            @keydown.escape="(e: KeyboardEvent) => !e.isComposing && emit('close')"
          />
        </div>
        <div :class="dialogFooterClass">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="text-prompt-cancel"
            @click="emit('close')"
            >{{
              t('common.action.cancel.label')
            }}</Button
          >
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="text-prompt-ok"
            @click="emit('submit')"
            >{{
              t('common.action.save.label')
            }}</Button
          >
        </div>
      </div>
    </FocusScope>
  </div>
</template>
