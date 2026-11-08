<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// A config file's problem on a tab without the file bar (P578). Nothing when the file is valid or
// the tab already shows this same problem.
type Problem = { line: number; message: string };
const props = defineProps<{
  error?: Problem | null;
  /** The problem the tab already shows, if any. */
  shown?: Problem | null;
  /** Switches to the YAML tab; omitted hides the action. */
  showYaml?: () => void;
}>();

const visible = computed(() => {
  const e = props.error;
  if (!e) return null;
  const s = props.shown;
  return s && s.line === e.line && s.message === e.message ? null : e;
});
</script>

<template>
  <Alert v-if="visible" variant="error" data-testid="config-file-problem">
    <AlertDescription>{{
      visible.line > 0
        ? t('common.configForm.problem.status', { line: visible.line, message: visible.message })
        : visible.message
    }}</AlertDescription>
    <AlertAction v-if="props.showYaml">
      <Button
        variant="link"
        size="inline"
        data-testid="config-file-problem-show-yaml"
        @click="props.showYaml"
      >
        {{ t('common.configForm.showYaml.label') }}
      </Button>
    </AlertAction>
  </Alert>
</template>
