<script setup lang="ts">
import type { ScriptParam } from '@shared/domain/scripts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import SecretInput from '@workbench/components/SecretInput.vue';
import { useId } from 'vue';

// Past this a toggle row gets too long; a select instead.
const MAX_TOGGLE_CHOICES = 5;
// SecondaryTabs drops an empty value, so None needs a non-empty one.
const NONE = '\u0000';

const values = defineModel<Record<string, string[]>>({ required: true });
const props = defineProps<{
  params: readonly ScriptParam[];
  /** Secret params with a saved value: left blank, the saved one is used. */
  saved?: readonly string[];
}>();
const idBase = useId();

function isSaved(p: ScriptParam): boolean {
  return p.secret && (props.saved ?? []).includes(p.name);
}

function set(name: string, next: string[]): void {
  values.value = { ...values.value, [name]: next };
}

function choices(p: ScriptParam): string[] {
  return p.required ? p.options : ['', ...p.options];
}

function selectItems(p: ScriptParam): { value: string; label: string; testid: string }[] {
  return choices(p).map((o) => ({
    value: o === '' ? NONE : o,
    label: o === '' ? t('common.state.none.option') : o,
    testid: `run-param-option-${p.name}-${o}`,
  }));
}

function toggle(p: ScriptParam, option: string, on: boolean): void {
  const cur = values.value[p.name] ?? [];
  set(
    p.name,
    p.options.filter((o) => (o === option ? on : cur.includes(o))),
  );
}
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="run-params">
    <Field v-for="p in params" :key="p.name" :data-testid="`run-param-${p.name}`">
      <FieldLabel :for="`${idBase}-${p.name}`">
        <span class="font-data">{{ p.name }}</span
        ><span v-if="p.required && !isSaved(p)" class="text-error">
          *<span class="sr-only"> {{ t('common.required.label') }}</span></span
        >
      </FieldLabel>
      <SecretInput
        v-if="p.type === 'text' && p.secret"
        :id="`${idBase}-${p.name}`"
        :model-value="values[p.name]?.[0] ?? ''"
        :set="isSaved(p) ? true : undefined"
        :placeholder="isSaved(p) ? t('automations.run.secretSaved.placeholder') : undefined"
        :data-testid="`run-param-input-${p.name}`"
        @update:model-value="(v) => set(p.name, v === '' ? [] : [v])"
      />
      <Input
        v-else-if="p.type === 'text'"
        :id="`${idBase}-${p.name}`"
        :model-value="values[p.name]?.[0] ?? ''"
        :data-testid="`run-param-input-${p.name}`"
        @update:model-value="(v) => set(p.name, String(v) === '' ? [] : [String(v)])"
      />
      <SecondaryTabs
        v-else-if="p.type === 'select' && choices(p).length <= MAX_TOGGLE_CHOICES"
        variant="choice"
        size="kira-lg"
        wrap
        :aria-label="p.name"
        :items="selectItems(p)"
        :model-value="values[p.name]?.[0] || NONE"
        :data-testid="`run-param-input-${p.name}`"
        @update:model-value="(v) => set(p.name, v === NONE ? [] : [v])"
      />
      <NativeSelect
        v-else-if="p.type === 'select'"
        :id="`${idBase}-${p.name}`"
        variant="bordered"
        size="kira-lg"
        :model-value="values[p.name]?.[0] ?? ''"
        :data-testid="`run-param-input-${p.name}`"
        @update:model-value="(v) => set(p.name, v === '' ? [] : [String(v)])"
      >
        <option value="" :disabled="p.required">{{ t('common.state.none.option') }}</option>
        <option v-for="o in p.options" :key="o" :value="o">{{ o }}</option>
      </NativeSelect>
      <div
        v-else
        class="flex flex-wrap gap-x-4 gap-y-1.5"
        :data-testid="`run-param-input-${p.name}`"
      >
        <div v-for="o in p.options" :key="o" class="flex items-center gap-2">
          <Checkbox
            :id="`${idBase}-${p.name}-${o}`"
            class="size-3.5"
            :model-value="(values[p.name] ?? []).includes(o)"
            :data-testid="`run-param-option-${p.name}-${o}`"
            @update:model-value="(v) => toggle(p, o, v === true)"
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          <Label :for="`${idBase}-${p.name}-${o}`">{{ o }}</Label>
        </div>
      </div>
    </Field>
  </div>
</template>
