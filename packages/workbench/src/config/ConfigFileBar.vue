<script setup lang="ts">
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';

// One config text's validity status (P450, P651). Paths and file actions are not shown; edits go
// through the app's form and YAML editors.
const props = defineProps<{
  error?: { line: number; message: string } | null;
}>();
</script>

<template>
  <div class="flex min-w-0 flex-1 flex-col gap-1" data-testid="config-file-bar">
    <FieldError v-if="props.error" class="font-data" data-testid="config-file-status">{{
      props.error.line > 0
        ? t('common.config.lineError.error', { line: props.error.line, text: props.error.message })
        : t('common.config.fileError.error', { text: props.error.message })
    }}</FieldError>
    <div v-else class="text-kira-sm text-ok" data-testid="config-file-status">
      {{ t('common.config.valid.status') }}
    </div>
  </div>
</template>
