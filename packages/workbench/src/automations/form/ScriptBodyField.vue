<script setup lang="ts">
import type { ScriptKind, ScriptParam } from '@shared/domain/scripts';
import ShortcutKbd from '@theme/components/ShortcutKbd.vue';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import AutocompleteField from '@workbench/components/AutocompleteField.vue';
import { braceToken, envToken } from '@workbench/editor/fieldCompletion';
import { computed } from 'vue';
import { scriptCandidates, scriptHighlights, scriptHover, usesParts } from '../scriptVars';

// The prompt (smart) or command: a multiline AutocompleteField that suggests the declared
// parameters and, in Kira Space, the ADE built-ins while typing `{name` or `$KIRA_…`.
const props = defineProps<{
  kind: ScriptKind;
  params: readonly ScriptParam[];
  ade: boolean;
  saved?: readonly string[];
  error?: string;
  errorId?: string;
}>();
const body = defineModel<string>({ required: true });

const isSmart = computed(() => props.kind === 'smart');
const src = computed(() => ({
  kind: props.kind,
  params: props.params,
  ade: props.ade,
  saved: props.saved,
}));
const candidates = computed(() => scriptCandidates(src.value));
const highlights = computed(() => scriptHighlights(src.value));
const hover = computed(() => scriptHover(src.value));
const parts = computed(() => usesParts(src.value, body.value));
const used = computed(() => parts.value.length > 1);
</script>

<template>
  <Field>
    <FieldLabel for="script-command">{{
      isSmart ? t('common.prompt.label') : t('common.field.command.label')
    }}</FieldLabel>
    <AutocompleteField
      id="script-command"
      v-model="body"
      multiline
      :rows="8"
      :auto-close="false"
      open-on-empty-token
      :candidates="candidates"
      :token-at="isSmart ? braceToken : envToken"
      :range-highlights="highlights"
      :hover-at="hover"
      :invalid="!!error"
      :aria-describedby="error ? errorId : undefined"
      :placeholder="
        isSmart
          ? t('automations.script.promptExample.example')
          : t('automations.script.commandExample.example')
      "
      class="w-full"
      data-testid="script-dialog-command"
    />
    <FieldError v-if="error" :id="errorId" data-testid="script-dialog-command-error">{{
      error
    }}</FieldError>
    <FieldDescription v-if="used" data-testid="script-dialog-uses"
      ><VarText :parts="parts" /></FieldDescription
    >
    <FieldDescription v-else-if="isSmart" data-testid="script-dialog-uses">
      <CatalogText k="automations.script.insertHint.hint" :params="{ token: '{name}' }">
        <template #chord><ShortcutKbd id="form.submit" /></template>
      </CatalogText>
    </FieldDescription>
    <FieldDescription v-else>
      <CatalogText k="automations.script.shellHint.hint">
        <template #chord><ShortcutKbd id="form.submit" /></template>
      </CatalogText>
    </FieldDescription>
  </Field>
</template>
