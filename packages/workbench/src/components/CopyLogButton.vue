<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useCopy } from '../behaviour/clipboard';
import { announceApp } from '../behaviour/liveRegion';
import { errorText } from '../bridge/codedError';
import { logPlainText } from '../lib/ansi';

export interface CopyLogSource {
  chunks: readonly { text: string }[];
  /** The log's head was dropped: the copy opens with a line saying so. */
  truncated: boolean;
}

// Copies a whole log: a drag selection holds only mounted virtual rows, and a cached page may hold
// less than the log, so `load` fetches what the copy needs on click.
const props = defineProps<{ load: () => Promise<CopyLogSource>; disabled?: boolean }>();

const { copy, copied } = useCopy();

async function onClick(): Promise<void> {
  let source: CopyLogSource;
  try {
    source = await props.load();
  } catch (err) {
    announceApp(t('common.copy.failed.error', { error: errorText(err) }));
    return;
  }
  const text = logPlainText(source.chunks);
  await copy(source.truncated ? `${t('common.copy.logTruncated.notice')}\n${text}` : text, {
    message: t('common.copy.copiedLog.announce'),
  });
}
</script>

<template>
  <Button variant="toolbar" size="kira" :disabled="disabled" @click="onClick">
    {{ copied ? t('common.copy.copiedLog.announce') : t('common.action.copyLog.label') }}
  </Button>
</template>
