<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useTimeoutFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { watch } from 'vue';
import { control } from '../bridge/control';
import { useProofWindowStore } from './state';

// Visible outcome of a screenshot save: where it went, with Reveal, or why it failed. A success
// hides itself; an error stays until dismissed or replaced by the next save.
const store = useProofWindowStore();

const hide = useTimeoutFn(() => store.clearShotResult(), TIMING.shotSavedStatus, {
  immediate: false,
});
watch(
  () => store.shotResult,
  (result) => {
    hide.stop();
    if (result?.kind === 'saved') hide.start();
  },
  { immediate: true },
);

async function reveal(): Promise<void> {
  const result = store.shotResult;
  if (result?.kind !== 'saved') return;
  try {
    await control.proofRevealShotFolder(result.folderId);
  } catch (e) {
    store.setShotResult({ kind: 'error', message: errorText(e) });
  }
}
</script>

<template>
  <div v-if="store.shotResult" class="shrink-0">
    <Alert
      strip
      :variant="store.shotResult.kind === 'saved' ? 'info' : 'error'"
      role="status"
      :data-tone="store.shotResult.kind"
      data-testid="proof-shot-status"
    >
      <AlertDescription>
        {{
          store.shotResult.kind === 'saved'
            ? t('studio.proof.shot.savedAll.status', {
                count: store.shotResult.count,
                path: store.shotResult.path,
              })
            : store.shotResult.message
        }}
      </AlertDescription>
      <AlertAction class="top-1/2 flex -translate-y-1/2 gap-2">
        <Button
          v-if="store.shotResult.kind === 'saved'"
          variant="link"
          size="inline"
          data-testid="proof-shot-reveal"
          @click="reveal()"
        >
          {{ t('common.action.reveal.label') }}
        </Button>
        <Button
          variant="link"
          size="inline"
          data-testid="proof-shot-status-dismiss"
          @click="store.clearShotResult()"
        >
          {{ t('common.action.dismiss.label') }}
        </Button>
      </AlertAction>
    </Alert>
  </div>
</template>
