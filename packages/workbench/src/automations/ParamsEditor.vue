<script setup lang="ts">
import type {
  ScriptKind,
  ScriptParam,
  ScriptParamType,
  SecretValues,
} from '@shared/domain/scripts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldDescription, FieldError, FieldLegend, FieldSet } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { useId } from 'vue';

import { paramEnvName } from './scriptVars';

const params = defineModel<ScriptParam[]>({ required: true });
// Write-only draft: name -> new value, or null to clear the saved one. Never read back.
const secretValues = defineModel<SecretValues>('secretValues', { default: () => ({}) });
const props = defineProps<{
  kind: ScriptKind;
  errors?: readonly (readonly string[])[];
  savedSecrets?: readonly string[];
  storageOff?: boolean;
}>();
const idBase = useId();
// A literal env var name, not copy; a catalog message can't hold `<>`.
const PARAM_VAR = '$KIRA_PARAM_<NAME>';

const TYPE_ITEMS = [
  { value: 'text', label: t('common.field.text.label'), testid: 'text' },
  { value: 'select', label: t('common.action.select.label'), testid: 'select' },
  {
    value: 'multiselect',
    label: t('automations.params.multiselect.option'),
    testid: 'multiselect',
  },
] as const;

function tokenHint(kind: ScriptKind, name: string): string {
  return kind === 'script' ? paramEnvName(name) : `{${name}}`;
}

function add(): void {
  params.value = [
    ...params.value,
    { name: '', type: 'text', options: [], default: [], required: false, secret: false },
  ];
}

function remove(i: number): void {
  params.value = params.value.filter((_, j) => j !== i);
}

function patch(i: number, change: Partial<ScriptParam>): void {
  params.value = params.value.map((p, j) => (j === i ? { ...p, ...change } : p));
}

function setType(i: number, type: ScriptParamType): void {
  patch(i, {
    type,
    options: type === 'text' ? [] : (params.value[i]?.options ?? []),
    default: [],
    secret: false,
  });
}

function isSaved(p: ScriptParam): boolean {
  return (
    p.name !== '' &&
    (props.savedSecrets ?? []).includes(p.name) &&
    secretValues.value[p.name] !== null
  );
}

function setSecret(name: string, value: string): void {
  if (name === '') return;
  const { [name]: _, ...rest } = secretValues.value;
  secretValues.value = value === '' ? rest : { ...rest, [name]: value };
}

function clearSecret(name: string): void {
  secretValues.value = { ...secretValues.value, [name]: null };
}

function addOption(i: number): void {
  patch(i, { options: [...(params.value[i]?.options ?? []), ''] });
}

function setOption(i: number, k: number, value: string): void {
  const p = params.value[i];
  if (!p) return;
  const old = p.options[k];
  patch(i, {
    options: p.options.map((o, j) => (j === k ? value : o)),
    default: p.default.map((d) => (d === old ? value : d)),
  });
}

function removeOption(i: number, k: number): void {
  const p = params.value[i];
  if (!p) return;
  const old = p.options[k];
  patch(i, {
    options: p.options.filter((_, j) => j !== k),
    default: p.default.filter((d) => d !== old),
  });
}

function moveOption(i: number, k: number, by: -1 | 1): void {
  const p = params.value[i];
  const to = k + by;
  if (!p || to < 0 || to >= p.options.length) return;
  const next = [...p.options];
  const [moved] = next.splice(k, 1);
  next.splice(to, 0, moved ?? '');
  patch(i, { options: next });
}

function toggleDefault(i: number, option: string, on: boolean): void {
  const p = params.value[i];
  if (!p) return;
  const rest = p.default.filter((d) => d !== option);
  patch(i, {
    default: p.type === 'multiselect' ? (on ? [...rest, option] : rest) : on ? [option] : [],
  });
}
</script>

<template>
  <FieldSet data-testid="params-editor">
    <div class="flex items-center gap-2">
      <FieldLegend class="mb-0">{{ t('automations.params.title.label') }}</FieldLegend>
      <Button
        variant="dialog"
        size="kira-lg"
        class="ml-auto"
        data-testid="param-add"
        @click="add"
        >{{
          t('automations.params.add.label')
        }}</Button
      >
    </div>
    <FieldDescription v-if="kind === 'script'">
      {{ t('automations.params.scriptHint.description', { variable: PARAM_VAR }) }}
    </FieldDescription>
    <FieldDescription v-else>
      {{ t('automations.params.smartHint.description', { token: '{name}', variable: PARAM_VAR }) }}
    </FieldDescription>
    <Card v-for="(p, i) in params" :key="i" data-testid="param-row">
      <div class="flex items-center gap-1.5">
        <Input
          :model-value="p.name"
          :placeholder="t('common.field.name.label')"
          class="min-w-0 flex-1 font-data"
          :data-testid="`param-name-${i}`"
          @update:model-value="(v) => patch(i, { name: String(v) })"
        />
        <SecondaryTabs
          variant="segmented"
          size="kira-lg"
          :model-value="p.type"
          :items="TYPE_ITEMS.map((it) => ({ ...it, testid: `param-type-${i}-${it.testid}` }))"
          @update:model-value="(v) => setType(i, v as ScriptParamType)"
        />
        <TooltipIconButton
          icon="trash"
          :label="t('automations.params.remove.tooltip', { name: p.name })"
          @click="remove(i)"
        />
      </div>
      <span
        v-if="p.name"
        class="font-data text-kira-sm text-muted-foreground"
        :data-testid="`param-token-${i}`"
        >{{
          tokenHint(kind, p.name)
        }}</span
      >
      <FieldError v-for="m in errors?.[i] ?? []" :key="m" :data-testid="`param-error-${i}`">{{
        m
      }}</FieldError>
      <Input
        v-if="p.type === 'text' && !p.secret"
        :model-value="p.default[0] ?? ''"
        :placeholder="t('common.state.default.option')"
        :data-testid="`param-default-${i}`"
        @update:model-value="(v) => patch(i, { default: String(v) === '' ? [] : [String(v)] })"
      />
      <div
        v-else-if="p.type === 'text'"
        class="flex flex-col gap-1"
        :data-testid="`param-secret-field-${i}`"
      >
        <div class="flex items-center gap-1.5">
          <Input
            type="password"
            autocomplete="new-password"
            :model-value="p.name ? (secretValues[p.name] ?? '') : ''"
            :disabled="storageOff || !p.name"
            :placeholder="
              isSaved(p)
                ? t('automations.params.secretValue.saved.placeholder')
                : t('common.field.value.label')
            "
            class="min-w-0 flex-1 font-data"
            :data-testid="`param-secret-value-${i}`"
            @update:model-value="(v) => setSecret(p.name, String(v))"
          />
          <Badge v-if="isSaved(p)" :data-testid="`param-secret-saved-${i}`">{{
            t('automations.params.secretValue.saved.status')
          }}</Badge>
          <Button
            v-if="isSaved(p)"
            variant="dialog"
            size="kira-lg"
            :data-testid="`param-secret-clear-${i}`"
            @click="clearSecret(p.name)"
            >{{
              t('common.action.clear.label')
            }}</Button
          >
        </div>
        <FieldDescription v-if="storageOff" :data-testid="`param-secret-off-${i}`">{{
          t('automations.params.secretValue.storageOff.description')
        }}</FieldDescription>
      </div>
      <div v-else class="flex flex-col gap-1" :data-testid="`param-options-${i}`">
        <div v-for="(o, k) in p.options" :key="k" class="flex items-center gap-1">
          <Checkbox
            class="size-3.5"
            :model-value="p.default.includes(o)"
            :aria-label="t('automations.params.default.aria', { option: o })"
            @update:model-value="(v) => toggleDefault(i, o, v === true)"
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          <Input
            :model-value="o"
            :placeholder="t('automations.params.option.placeholder')"
            class="min-w-0 flex-1"
            :data-testid="`param-option-${i}-${k}`"
            @update:model-value="(v) => setOption(i, k, String(v))"
          />
          <TooltipIconButton
            icon="arrow-up"
            :label="t('common.moveUp.tooltip')"
            @click="moveOption(i, k, -1)"
          />
          <TooltipIconButton
            icon="arrow-down"
            :label="t('common.moveDown.tooltip')"
            @click="moveOption(i, k, 1)"
          />
          <TooltipIconButton
            icon="close"
            :label="t('automations.params.removeOption.tooltip')"
            @click="removeOption(i, k)"
          />
        </div>
        <Button
          variant="dialog"
          size="kira-lg"
          class="self-start"
          :data-testid="`param-option-add-${i}`"
          @click="addOption(i)"
        >
          {{ t('automations.params.addOption.label') }}
        </Button>
      </div>
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-1.5">
          <Checkbox
            :id="`${idBase}-req-${i}`"
            class="size-3.5"
            :model-value="p.required"
            @update:model-value="(v) => patch(i, { required: v === true })"
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          <Label :for="`${idBase}-req-${i}`">{{ t('common.required.label') }}</Label>
        </div>
        <div v-if="p.type === 'text'" class="flex items-center gap-1.5">
          <Checkbox
            :id="`${idBase}-sec-${i}`"
            class="size-3.5"
            :model-value="p.secret"
            :data-testid="`param-secret-${i}`"
            @update:model-value="
              (v) => patch(i, v === true ? { secret: true, default: [] } : { secret: false })
            "
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          <Label :for="`${idBase}-sec-${i}`">{{ t('common.secret.label') }}</Label>
        </div>
      </div>
    </Card>
  </FieldSet>
</template>
