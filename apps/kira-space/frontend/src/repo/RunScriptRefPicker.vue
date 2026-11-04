<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import { Button } from '@theme/components/ui/button';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import ParamsForm from '@workbench/automations/run/ParamsForm.vue';
import { computed } from 'vue';
import { useCustomScriptsStore } from '../state/customScripts';
import type { RepoScriptRefValue } from './state/repoFormTypes';
import {
  groupScripts,
  normalScripts,
  refParamsFromForm,
  refParamsToForm,
  refStatus,
} from './state/scriptRef';

// Picks the Run script a repo file runs. Normal scripts only: a repo file refuses a smart script.
// The status lines mirror the run-time refusals and never block Save.
const props = defineProps<{ id: string; testid: string; place: 'worktree' | 'repo' }>();
const value = defineModel<RepoScriptRefValue | undefined>({ required: true });

const scripts = useCustomScriptsStore();
const normal = computed(() => normalScripts(scripts.records));
const grouped = computed(() => groupScripts(normal.value));
const named = computed(() => value.value?.script ?? '');
const listed = computed(() => normal.value.some((s) => s.name === named.value));
const status = computed(() => (value.value ? refStatus(scripts.records, value.value) : undefined));
const script = computed(() => (status.value?.kind === 'ok' ? status.value.script : undefined));
const plain = computed(() => script.value?.params.filter((p) => !p.secret) ?? []);
const secrets = computed(() => script.value?.params.filter((p) => p.secret) ?? []);
const secretNames = computed(() => secrets.value.map((p) => p.name).join(', '));
const unsaved = computed(() =>
  secrets.value
    .filter((p) => p.required && !script.value?.savedSecrets.includes(p.name))
    .map((p) => p.name)
    .join(', '),
);
const bad = computed(() => {
  const s = status.value;
  if (s?.kind !== 'ok') return [];
  return [...new Set([...s.unknown, ...s.secretGiven])];
});

function pick(name: string): void {
  value.value = name === '' ? undefined : { script: name };
}
function setParams(values: Record<string, string[]>): void {
  if (!script.value) return;
  const params = refParamsFromForm(values, script.value);
  value.value = { script: named.value, ...(params && { params }) };
}
function removeBad(): void {
  setParams(refParamsToForm(value.value?.params));
}
const dirHint = computed(() => {
  if (!script.value) return '';
  if (!script.value.useAdeDir)
    return t('space.repo.config.scriptRef.dirOwn.hint', { variable: 'KIRA_WORKTREE_PATH' });
  return t(
    props.place === 'worktree'
      ? 'space.repo.config.scriptRef.dirWorktree.hint'
      : 'space.repo.config.scriptRef.dirRepo.hint',
  );
});
</script>

<template>
  <InlineState
    v-if="normal.length === 0 && !value"
    kind="empty"
    :data-testid="`${testid}-run-empty`"
  >
    {{ t('space.repo.config.scriptRef.empty') }}
  </InlineState>
  <div v-else class="flex min-w-0 flex-col gap-2">
    <Field>
      <FieldLabel :for="id">{{ t('space.repo.config.scriptRef.script.label') }}</FieldLabel>
      <NativeSelect
        :id="id"
        variant="bordered"
        class="w-full"
        :model-value="named"
        :data-testid="`${testid}-run-script`"
        @update:model-value="(v) => pick(String(v))"
      >
        <option value="">{{ t('common.state.none.option') }}</option>
        <option v-if="named && !listed" :value="named" disabled>{{ named }}</option>
        <option v-for="s in grouped.ungrouped" :key="s.id" :value="s.name">{{ s.name }}</option>
        <optgroup v-for="g in grouped.groups" :key="g.collection" :label="g.collection">
          <option v-for="s in g.scripts" :key="s.id" :value="s.name">{{ s.name }}</option>
        </optgroup>
      </NativeSelect>
    </Field>
    <FieldError v-if="status?.kind === 'smart'" :data-testid="`${testid}-run-error`">
      {{ t('space.repo.config.scriptRef.smart.error', { script: named }) }}
    </FieldError>
    <FieldError v-else-if="status?.kind === 'missing'" :data-testid="`${testid}-run-error`">
      {{ t('space.repo.config.scriptRef.missing.error', { script: named }) }}
    </FieldError>
    <FieldError v-else-if="status?.kind === 'duplicate'" :data-testid="`${testid}-run-error`">
      {{ t('space.repo.config.scriptRef.duplicate.error', { count: status.count, script: named }) }}
    </FieldError>
    <template v-else-if="status?.kind === 'ok'">
      <FieldError v-if="status.unknown.length > 0" :data-testid="`${testid}-run-error`">
        {{
          t('space.repo.config.scriptRef.unknownParams.error', {
            script: named,
            names: status.unknown.join(', '),
          })
        }}
      </FieldError>
      <FieldError v-if="status.secretGiven.length > 0" :data-testid="`${testid}-run-error`">
        {{
          t('space.repo.config.scriptRef.secretGiven.error', {
            names: status.secretGiven.join(', '),
          })
        }}
      </FieldError>
      <Button
        v-if="bad.length > 0"
        variant="link"
        size="inline"
        class="self-start"
        :data-testid="`${testid}-run-unknown-remove`"
        @click="removeBad"
      >
        {{ t('common.action.remove.label') }}
      </Button>
      <div
        v-if="plain.length > 0"
        class="flex flex-col gap-1"
        :data-testid="`${testid}-run-params`"
      >
        <ParamsForm
          :model-value="refParamsToForm(value?.params)"
          :params="plain"
          @update:model-value="setParams"
        />
        <FieldDescription>{{ t('space.repo.config.scriptRef.defaults.hint') }}</FieldDescription>
      </div>
      <FieldDescription v-if="secretNames" :data-testid="`${testid}-run-secret`">
        {{ t('space.repo.config.scriptRef.secretParams.hint', { names: secretNames }) }}
        <template v-if="unsaved">
          {{ t('space.repo.config.scriptRef.secretUnsaved.hint', { names: unsaved }) }}
        </template>
      </FieldDescription>
      <FieldDescription :data-testid="`${testid}-run-dir`">{{ dirHint }}</FieldDescription>
    </template>
  </div>
</template>
