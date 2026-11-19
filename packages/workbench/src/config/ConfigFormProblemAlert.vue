<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';

// A config form's own problem (the form cannot represent the file): its line and message, with a
// Show YAML action when `showYaml` is set. Attrs (`strip`, class) reach the Alert.
defineProps<{
  problem: { line?: number; message: string };
  testid: string;
  showYaml?: boolean;
  showYamlTestid?: string;
}>();
const emit = defineEmits<{ showYaml: [] }>();
</script>

<template>
  <Alert variant="error" :data-testid="testid">
    <AlertDescription>{{
      t('common.configForm.problem.status', { line: problem.line ?? 0, message: problem.message })
    }}</AlertDescription>
    <AlertAction v-if="showYaml">
      <Button
        variant="link"
        size="inline"
        :data-testid="showYamlTestid ?? `${testid}-show-yaml`"
        @click="emit('showYaml')"
      >
        {{ t('common.configForm.showYaml.label') }}
      </Button>
    </AlertAction>
  </Alert>
</template>
