<script setup lang="ts">
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { ref } from 'vue';
import { useCopy } from '../behaviour/clipboard';
import { errorText } from '../bridge/codedError';

// One config file's path, validity and the plain-file actions every config module shares (P450). The
// app passes the bound calls in; this component never imports a binding.
const props = defineProps<{
  path: string;
  error?: { line: number; message: string } | null;
  open: () => Promise<unknown>;
  reveal: () => Promise<unknown>;
  /** Opens the folder's AGENTS.md; omitted when the folder has none. */
  about?: () => Promise<unknown>;
}>();

const { copy, copied } = useCopy();
const actionError = ref<string | null>(null);

async function run(call: () => Promise<unknown>, fail: (text: string) => string): Promise<void> {
  actionError.value = null;
  try {
    await call();
  } catch (err) {
    actionError.value = fail(errorText(err));
  }
}
</script>

<template>
  <div class="flex min-w-0 flex-1 flex-col gap-1" data-testid="config-file-bar">
    <div class="flex min-w-0 flex-wrap items-center gap-2">
      <TruncatedText
        :text="props.path"
        split="path"
        class="min-w-0 flex-1 font-data text-kira-sm text-subtle"
        data-testid="config-file-path"
      />
      <Button
        variant="dialog"
        size="kira-lg"
        data-testid="config-file-open"
        @click="run(props.open, (text) => t('common.config.open.failed.error', { text }))"
      >
        {{ t('common.action.open.label') }}
      </Button>
      <Button
        variant="dialog"
        size="kira-lg"
        data-testid="config-file-reveal"
        @click="run(props.reveal, (text) => t('common.config.reveal.failed.error', { text }))"
      >
        {{ t('common.action.reveal.label') }}
      </Button>
      <Button
        variant="dialog"
        size="kira-lg"
        data-testid="config-file-copy-path"
        @click="copy(props.path)"
      >
        {{ copied ? t('common.copy.copiedPath.announce') : t('common.action.copyPath.label') }}
      </Button>
      <Button
        v-if="props.about"
        variant="link"
        size="inline"
        data-testid="config-file-about"
        @click="run(props.about, (text) => t('common.config.open.failed.error', { text }))"
      >
        {{ t('common.config.about.label') }}
      </Button>
    </div>
    <FieldError v-if="props.error" class="font-data" data-testid="config-file-status">{{
      props.error.line > 0
        ? t('common.config.lineError.error', { line: props.error.line, text: props.error.message })
        : t('common.config.fileError.error', { text: props.error.message })
    }}</FieldError>
    <div v-else class="text-kira-sm text-ok" data-testid="config-file-status">
      {{ t('common.config.valid.status') }}
    </div>
    <FieldError v-if="actionError" data-testid="config-file-action-error">{{
      actionError
    }}</FieldError>
  </div>
</template>
