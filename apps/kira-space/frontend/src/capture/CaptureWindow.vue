<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Kbd } from '@theme/components/ui/kbd';
import { t } from '@theme/i18n';
import { tryOnScopeDispose, useEventListener, useWindowFocus } from '@vueuse/core';
import { useFocusOnMount } from '@workbench/behaviour/focus';
import { errorText } from '@workbench/bridge/codedError';
import { windowKey } from '@workbench/bridge/rpc';
import { formatShortcut, shortcutFor } from '@workbench/shortcuts/keys';
import { type ComponentPublicInstance, computed, ref, useTemplateRef, watch } from 'vue';
import { localIso } from '../ade/v2/localDay';
import { control } from '../bridge/control';
import { useConfirmCapture, useDismissCapture } from './queries';
import { useCaptureWindowStore } from './state';
import type { CaptureItem } from './wire';

// The quick-capture popup: what the clipboard held. Enter adds it to Later, CmdOrCtrl+Enter to
// Today; Esc or losing focus closes.
const store = useCaptureWindowStore();
const item = computed(() => store.item as CaptureItem);

const title = ref('');
const notice = ref('');
const confirm = useConfirmCapture();
const dismiss = useDismissCapture();

// A second hotkey press pushes its item here; it replaces the content.
tryOnScopeDispose(control.onCaptureItem((next) => (store.item = next)));

// A pushed item replaces the content and clears the last error.
watch(
  item,
  (next) => {
    title.value = next.title;
    notice.value = '';
    confirm.reset();
  },
  { immediate: true },
);

const isEmpty = computed(() => item.value.kind === 'empty');
const isChangeRequest = computed(() => item.value.kind === 'changeRequest');
const error = computed(() => (confirm.error.value ? errorText(confirm.error.value) : ''));

function close(): void {
  dismiss.mutate(windowKey);
}

function add(day: string | null): void {
  if (isEmpty.value || notice.value || confirm.isPending.value) return;
  confirm.mutate(
    { windowKey, itemId: item.value.id, title: title.value, day },
    { onSuccess: (r) => (notice.value = r.notice) },
  );
}

useEventListener(window, 'keydown', (e) => {
  if (e.isComposing) return;
  if (e.key === 'Escape') close();
  else if (e.key !== 'Enter') return;
  else if (isEmpty.value || notice.value) close();
  else if (shortcutFor(e, ['form.submit'])) add(localIso(new Date()));
  else if (!e.shiftKey && !e.altKey) add(null);
  e.preventDefault();
});

// Losing focus after having it dismisses; a panel that never got focus stays.
const focused = useWindowFocus();
watch(focused, (now, before) => {
  if (before && !now && !confirm.isPending.value) close();
});

// A file dropped on the popup must not navigate the webview to it.
useEventListener(window, ['dragover', 'drop'], (e) => e.preventDefault());

const titleInput = useTemplateRef<ComponentPublicInstance>('titleInput');
useFocusOnMount(titleInput);
</script>

<template>
  <div
    class="flex h-full flex-col gap-2 bg-chrome p-3 text-fg"
    :aria-label="t('space.capture.window.label')"
    data-testid="capture-window"
  >
    <template v-if="isEmpty">
      <FieldDescription data-testid="capture-empty">{{
        t('space.capture.empty.body')
      }}</FieldDescription>
      <div class="mt-auto flex justify-end">
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="capture-dismiss"
          @click="close"
        >
          {{ t('common.action.dismiss.label') }}
        </Button>
      </div>
    </template>
    <template v-else>
      <div v-if="item.label" class="flex items-center gap-2">
        <Badge data-testid="capture-label">{{ item.label }}</Badge>
      </div>
      <pre
        class="max-h-28 overflow-hidden whitespace-pre-wrap break-all font-ui text-kira-md"
        data-testid="capture-text"
      >{{ item.text }}</pre>
      <FieldDescription v-if="item.truncated" data-testid="capture-truncated">{{
        t('space.capture.truncated.body')
      }}</FieldDescription>
      <Input
        ref="titleInput"
        v-model="title"
        :disabled="confirm.isPending.value"
        :aria-label="t('space.capture.title.label')"
        :placeholder="t('space.capture.title.label')"
        data-testid="capture-title"
      />
      <FieldDescription
        v-if="isChangeRequest && confirm.isPending.value"
        data-testid="capture-pending"
        >{{
          t('space.capture.fetching.body')
        }}</FieldDescription
      >
      <FieldDescription v-else-if="isChangeRequest && !notice" data-testid="capture-review">{{
        t('space.capture.review.body')
      }}</FieldDescription>
      <Alert v-if="error" variant="error" data-testid="capture-error">
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <Alert v-if="notice" variant="warn" data-testid="capture-notice">
        <AlertDescription>{{ notice }}</AlertDescription>
      </Alert>
      <FieldDescription class="mt-auto flex items-center justify-end gap-3">
        <template v-if="notice">
          <span
            ><Kbd>{{ t('common.key.enter.label') }}</Kbd>
            {{ t('common.action.close.label') }}</span
          >
        </template>
        <template v-else>
          <span data-testid="capture-hint-later"
            ><Kbd>{{ t('common.key.enter.label') }}</Kbd> {{ t('common.later.label') }}</span
          >
          <span data-testid="capture-hint-today"
            ><Kbd>{{ formatShortcut('form.submit') }}</Kbd>
            {{ t('space.common.day.today.label') }}</span
          >
        </template>
        <span
          ><Kbd>{{ t('common.key.escape.label') }}</Kbd>
          {{ t('common.action.close.label') }}</span
        >
      </FieldDescription>
    </template>
  </div>
</template>
