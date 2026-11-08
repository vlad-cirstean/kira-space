<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useCopy } from '../behaviour/clipboard';
import { logPlainText } from '../lib/ansi';

// Copies a whole log from its loaded chunks: a drag selection holds only mounted virtual rows.
const props = defineProps<{ chunks: readonly { text: string }[] }>();

const { copy, copied } = useCopy();

function onClick(): void {
  void copy(logPlainText(props.chunks), { message: t('common.copy.copiedLog.announce') });
}
</script>

<template>
  <Button variant="toolbar" size="kira" :disabled="chunks.length === 0" @click="onClick">
    {{ copied ? t('common.copy.copiedLog.announce') : t('common.action.copyLog.label') }}
  </Button>
</template>
