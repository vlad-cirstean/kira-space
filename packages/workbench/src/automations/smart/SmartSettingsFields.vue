<script setup lang="ts">
import { DEFAULT_SMART, type SmartSettings } from '@shared/domain/scripts';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useAgentInfo } from '@workbench/agent/agentInfo';

const smart = defineModel<SmartSettings>({ required: true });
const { info } = useAgentInfo();

function onNoCostCap(on: boolean): void {
  smart.value.noCostCap = on;
  if (!on && !(smart.value.maxBudgetUsd > 0)) smart.value.maxBudgetUsd = DEFAULT_SMART.maxBudgetUsd;
}
</script>

<template>
  <div class="flex items-end gap-3" data-testid="smart-settings">
    <Field class="min-w-0 flex-1">
      <FieldLabel for="smart-model">{{ t('automations.smart.model.label') }}</FieldLabel>
      <NativeSelect
        id="smart-model"
        v-model="smart.model"
        variant="bordered"
        size="kira-lg"
        data-testid="smart-model"
      >
        <option v-for="m in info.models" :key="m.id" :value="m.id">{{ m.id }}</option>
      </NativeSelect>
    </Field>
    <Field class="w-32">
      <FieldLabel for="smart-budget">{{ t('automations.smart.budget.label') }}</FieldLabel>
      <Input
        id="smart-budget"
        v-model.number="smart.maxBudgetUsd"
        type="number"
        min="0.01"
        step="0.1"
        :disabled="smart.noCostCap"
        data-testid="smart-budget"
      />
    </Field>
    <Field orientation="horizontal" class="w-auto shrink-0 items-center pb-2">
      <Checkbox
        id="smart-no-cost-cap"
        :model-value="smart.noCostCap"
        data-testid="smart-no-cost-cap"
        @update:model-value="(v) => onNoCostCap(v === true)"
      />
      <Label for="smart-no-cost-cap">{{ t('automations.smart.noCostCap.label') }}</Label>
    </Field>
    <Field class="w-28">
      <FieldLabel for="smart-timeout">{{ t('common.field.timeout.label') }}</FieldLabel>
      <Input
        id="smart-timeout"
        v-model="smart.timeout"
        :placeholder="t('automations.smart.timeoutExample.example')"
        class="font-data"
        data-testid="smart-timeout"
      />
    </Field>
  </div>
</template>
