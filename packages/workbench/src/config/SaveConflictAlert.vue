<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';

// A failed save of a config file. On a conflict (the file changed on disk) the alert offers Refresh
// (`reload`) and Overwrite. `testidPrefix` gives `<prefix>-save-error`, `-reload` and `-overwrite`;
// the default slot replaces the error text; `variant` is `error` unless the host treats it as a
// warning. Attrs (`strip`, class) reach the Alert.
withDefaults(
  defineProps<{
    error: string;
    conflict: boolean;
    testidPrefix: string;
    variant?: 'error' | 'warn';
  }>(),
  { variant: 'error' },
);
const emit = defineEmits<{ reload: []; overwrite: [] }>();
</script>

<template>
  <Alert v-if="error" :variant="variant" :data-testid="`${testidPrefix}-save-error`">
    <AlertDescription><slot>{{ error }}</slot></AlertDescription>
    <AlertAction v-if="conflict">
      <Button
        variant="link"
        size="inline"
        :data-testid="`${testidPrefix}-reload`"
        @click="emit('reload')"
      >
        {{ t('common.action.refresh.label') }}
      </Button>
      <Button
        variant="link"
        size="inline"
        :data-testid="`${testidPrefix}-overwrite`"
        @click="emit('overwrite')"
      >
        {{ t('common.action.overwrite.label') }}
      </Button>
    </AlertAction>
  </Alert>
</template>
