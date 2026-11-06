<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { refDebounced } from '@vueuse/core';
import { useFocusOnMount } from '@workbench/behaviour/focus';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, ref, useTemplateRef } from 'vue';
import { useImportCurlStore } from './state/curl';

const importCurlStore = useImportCurlStore();

// Paste, live preview, live warnings, Import. A plain <textarea> rather than CodeMirror —
// there is no grammar to highlight and the bundle argument applies. Everything below the
// textarea recomputes on every input (parseCurl is pure and synchronous) — the warnings are
// shown *before* Import is pressed, deliberately diverging from ImportReportStrip's post-hoc
// report: a curl paste is a short string sitting right there, so the honest place to say
// "-k was ignored" is beside it, while it can still be edited.
const text = ref('');

// For a large pasted curl command (a big JSON body, say), re-lexing the whole text on
// every single keystroke to update a one-line summary has no business running that often —
// the textarea itself (`text`) stays bound immediately so typing never stutters; the preview below
// reads a debounced copy instead (TIMING.preview, shared with every other parse preview).
// Unlike EditRawRequestDialog.vue's own copy, this one has no external-open reset to preserve, so
// refDebounced is a direct fit — no manual watch/setTimeout needed.
const debouncedText = refDebounced(text, TIMING.preview);

const preview = computed(() => importCurlStore.previewCurl(debouncedText.value));

function onImport(): void {
  importCurlStore.submitImportCurl(text.value);
}

const curlExample = t('studio.api.importCurl.example', { json: '{"id": 1}' });

function close(): void {
  importCurlStore.closeImportCurlDialog();
}

// P105 §8: reka-ui's Dialog, unlike KuiDialog, auto-focuses nothing on its own.
const curlEl = useTemplateRef<HTMLElement>('curlEl');
useFocusOnMount(curlEl);
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && close()">
    <DialogContent size="lg" :dirty="text.trim() !== ''" data-testid="import-curl-dialog">
      <DialogHeader close-testid="import-curl-dialog-close">
        <DialogTitle>{{ t('studio.api.importCurl.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Textarea
          ref="curlEl"
          v-model="text"
          class="font-data curl-textarea min-h-32"
          rows="6"
          :placeholder="curlExample"
          data-testid="import-curl-textarea"
        />

        <Alert v-if="preview.error" variant="error" data-testid="import-curl-error">
          <AlertDescription>{{ preview.error }}</AlertDescription>
        </Alert>
        <template v-else-if="text.trim() !== ''">
          <FieldDescription data-testid="import-curl-summary">{{
            preview.summary
          }}</FieldDescription>
          <!-- D12: the same `warn` variant + `<li :data-kind>` shape ImportReportStrip.vue
             established — shown live, before Import is pressed, rather than as a post-hoc report. -->
          <Alert
            v-if="preview.warnings.length > 0"
            variant="warn"
            data-testid="import-curl-warnings"
          >
            <AlertDescription>
              <ul class="m-0 flex flex-col gap-0.5 pl-2">
                <li v-for="(warning, i) in preview.warnings" :key="i" :data-kind="warning.kind">
                  {{ t(warning.detail.key, warning.detail.params) }}
                </li>
              </ul>
            </AlertDescription>
          </Alert>
        </template>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="import-curl-cancel" @click="close">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="import-curl-submit"
          :disabled="text.trim() === '' || preview.error !== null"
          @click="onImport"
        >
          {{ t('common.import.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
