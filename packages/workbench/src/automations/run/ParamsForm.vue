<script setup lang="ts">
import type { ScriptParam } from '@shared/domain/scripts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { t } from '@theme/i18n';
import { useId } from 'vue';

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
      <Input
        v-if="p.type === 'text'"
        :id="`${idBase}-${p.name}`"
        :type="p.secret ? 'password' : 'text'"
        :model-value="values[p.name]?.[0] ?? ''"
        :autocomplete="p.secret ? 'off' : undefined"
        :placeholder="isSaved(p) ? t('automations.run.secretSaved.placeholder') : undefined"
        :data-testid="`run-param-input-${p.name}`"
        @update:model-value="(v) => set(p.name, String(v) === '' ? [] : [String(v)])"
      />
      <RadioGroup
        v-else-if="p.type === 'select'"
        :model-value="values[p.name]?.[0] ?? ''"
        class="flex flex-row flex-wrap gap-x-4 gap-y-1.5"
        :data-testid="`run-param-input-${p.name}`"
        @update:model-value="(v) => set(p.name, v === '' ? [] : [String(v)])"
      >
        <div v-for="o in choices(p)" :key="o" class="flex items-center gap-2">
          <RadioGroupItem
            :id="`${idBase}-${p.name}-${o}`"
            :value="o"
            :data-testid="`run-param-option-${p.name}-${o}`"
          />
          <Label :for="`${idBase}-${p.name}-${o}`" :class="o === '' && 'text-muted-foreground'">{{
            o === '' ? t('common.state.none.option') : o
          }}</Label>
        </div>
      </RadioGroup>
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
