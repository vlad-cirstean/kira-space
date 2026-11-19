<script setup lang="ts">
import { FILE_KINDS } from '@shared/domain/connection';
import Disclosure from '@theme/components/Disclosure.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { joinSentences, t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import { regenerateMaskKey } from '../../state/maskRules';

// The secret key behind matching tags. Regenerating takes effect at once, outside the file draft.
const props = defineProps<{ connectionId: string; kind: string }>();
const confirmDialogStore = useConfirmDialogStore();
const connectionsStore = useConnectionsStore();

const isFileStyle = computed(() => FILE_KINDS.has(props.kind as never));
const secretStatus = computed(() => connectionsStore.secretStorage);
const regenerate = useAsyncAction(() => regenerateMaskKey(props.connectionId));

async function onRegenerateKey(): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('studio.project.connectionDialog.regenerateConfirm.title'),
    message: joinSentences(
      t('studio.project.connectionDialog.regenerateConfirm.body'),
      t('common.confirm.irreversible.body'),
    ),
    confirmLabel: t('studio.common.regenerate.label'),
    tone: 'danger',
  });
  if (!ok) return;
  await regenerate.run();
}
</script>

<template>
  <Disclosure
    :label="t('studio.project.connectionDialog.privacy.key.title')"
    data-testid="mask-key-section"
  >
    <FieldDescription class="w-full">{{
      t('studio.project.connectionDialog.privacy.key.description')
    }}</FieldDescription>
    <FieldDescription v-if="!connectionId" class="w-full">{{
      t('studio.project.connectionDialog.privacy.saveFirst.hint')
    }}</FieldDescription>
    <Button
      v-else
      variant="dialog"
      size="kira-lg"
      class="self-start"
      data-testid="mask-regenerate-key"
      @click="onRegenerateKey"
    >
      {{ t('studio.project.connectionDialog.regenerate.label') }}
    </Button>
    <Alert v-if="regenerate.error.value" variant="error" data-testid="mask-key-error">
      <AlertDescription>{{ regenerate.error.value }}</AlertDescription>
    </Alert>
    <Alert
      v-if="!isFileStyle && secretStatus?.insecureFallback"
      data-testid="mask-key-credential-note"
      variant="warn"
    >
      <AlertDescription>
        {{ t('studio.project.connectionDialog.privacy.keyFallback.body') }}
      </AlertDescription>
    </Alert>
  </Disclosure>
</template>
