<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
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
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed } from 'vue';
import { useCopyAsCurlStore } from './state/curl';

const copyAsCurlStore = useCopyAsCurlStore();

// The generated command, masked by default. Secrets are still {{token}} until Show secret
// values is pressed — the gate is `revealSecretValues`'s own `revealVariable` calls
// (http/state/variables.ts's existing four-outcome flow), not anything here. Built on ui/dialog
// + the shared Alert `note`/`warn` variants (ImportReportStrip.vue's own technique).
const command = computed(() => copyAsCurlStore.currentCurlCommand());

const maskedNames = computed(() =>
  copyAsCurlStore.deferredNames.filter(
    (name) => copyAsCurlStore.revealedSecretValues[name] === undefined,
  ),
);
const hasRevealedAny = computed(() => Object.keys(copyAsCurlStore.revealedSecretValues).length > 0);

const stripTone = computed<'info' | 'warn'>(() => (hasRevealedAny.value ? 'warn' : 'info'));
// D10 step 5: "the strip says which names are still masked" — true whether nothing has been
// revealed yet, or a reveal for one name succeeded while another was cancelled/errored/declined.
const stripText = computed(() => {
  const masked = maskedNames.value;
  if (masked.length === 0) return t('studio.api.curl.realSecrets.body');
  const names = masked.join(', ');
  return t(
    hasRevealedAny.value
      ? 'studio.api.curl.secretsStillHidden.body'
      : 'studio.api.curl.secretsHidden.body',
    { count: masked.length, names },
  );
});

// P6 D12 fact 2: {{$…}} values are frozen into the command on open (D10 whole reason to exist) —
// a `resolved` ref whose name still starts with '$' is exactly a generated dynamic value, since a
// deferred (secret) ref never reaches 'resolved' until applySecretValues fills it in later.
const hasDynamicValue = computed(() =>
  (copyAsCurlStore.resolved?.refs ?? []).some(
    (r) => r.kind === 'resolved' && r.name.startsWith('$'),
  ),
);

const reveal = useAsyncAction(() => copyAsCurlStore.revealSecretValues());

function onCopy(): void {
  copyAsCurlStore.copyCurlCommand();
}

function close(): void {
  copyAsCurlStore.closeCopyAsCurlDialog();
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && close()">
    <DialogContent size="xl" data-testid="copy-as-curl-dialog">
      <DialogHeader close-testid="copy-as-curl-dialog-close">
        <DialogTitle>{{ t('studio.api.copyAsCurl.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Textarea
          class="font-data min-h-44"
          :model-value="command"
          readonly
          rows="10"
          data-testid="copy-as-curl-command"
        />

        <Alert
          v-if="copyAsCurlStore.deferredNames.length > 0"
          :variant="stripTone"
          data-testid="copy-as-curl-strip"
        >
          <AlertDescription>{{ stripText }}</AlertDescription>
          <AlertAction v-if="maskedNames.length > 0 || hasRevealedAny">
            <Button
              v-if="hasRevealedAny"
              variant="dialog"
              size="kira"
              data-testid="copy-as-curl-hide"
              @click="copyAsCurlStore.hideSecretValues()"
            >
              {{ t('studio.api.copyAsCurl.hide.label') }}
            </Button>
            <Button
              v-if="maskedNames.length > 0"
              variant="dialog"
              size="kira"
              :pending="reveal.pending.value"
              data-testid="copy-as-curl-reveal"
              @click="reveal.run"
            >
              {{ t('studio.api.copyAsCurl.reveal.label') }}
            </Button>
          </AlertAction>
        </Alert>

        <FieldDescription v-if="hasDynamicValue" data-testid="copy-as-curl-dynamic-note">
          {{ t('studio.api.curl.dynamicValues.hint') }}
        </FieldDescription>

        <Alert v-if="copyAsCurlStore.error" variant="error" data-testid="copy-as-curl-error">
          <AlertDescription>{{ copyAsCurlStore.error }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="copy-as-curl-close" @click="close">{{
          t('common.action.close.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="copy-as-curl-copy"
          @click="onCopy"
        >
          {{ t('common.action.copy.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
