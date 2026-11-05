<script setup lang="ts">
import type { ApiFileVariable } from '@shared/domain/apiConfig';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { computed, ref } from 'vue';
import { applyBulk, serializeVariables } from './variablesForm';

// The `.env` view of the variables list. The text is seeded once from the rows the editor opened on;
// every valid edit replaces the rows in the draft, matched to those opening rows by name, so a secret
// keeps its reference. A value on a secret's line is refused: secrets are entered in the row's SecretInput.
const props = defineProps<{ variables: ApiFileVariable[] }>();
const emit = defineEmits<{ 'update:variables': [ApiFileVariable[]] }>();

const baseline = props.variables.map((v) => ({ ...v }));
const text = ref(serializeVariables(baseline));
const result = ref(applyBulk(baseline, text.value));
const error = computed(() => result.value.error);

function onInput(next: string): void {
  text.value = next;
  result.value = applyBulk(baseline, next);
  if (result.value.error) return;
  emit('update:variables', result.value.variables);
}
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="variables-bulk-editor">
    <FieldDescription data-testid="variables-bulk-hint">{{
      t('studio.api.variablesDialog.bulk.hint')
    }}</FieldDescription>
    <CodeTextEditor
      :model-value="text"
      language="plain"
      :ariaLabel="t('studio.api.variablesDialog.bulk.toggle.label')"
      class="h-64"
      data-testid="variables-bulk-text"
      @update:model-value="onInput"
    />
    <Alert v-if="error" variant="error" data-testid="variables-bulk-error">
      <AlertDescription>{{
        t('common.configForm.problem.status', {
          line: error.line,
          message: t(error.message.key, error.message.params),
        })
      }}</AlertDescription>
    </Alert>
    <FieldDescription v-else data-testid="variables-bulk-summary">{{
      t('studio.api.variablesDialog.bulk.summary.status', result.summary)
    }}</FieldDescription>
  </div>
</template>
