<script setup lang="ts">
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import { toNumber } from '../lib/editDiff';

defineOptions({ inheritAttrs: false });
const model = defineModel<number>({ required: true });
defineProps<{
  allowUnlimited?: boolean;
  disabled?: boolean;
  testid: string;
}>();
const MIB = 1024 ** 2;
const GIB = 1024 ** 3;

const unit = ref<'MB' | 'GB'>(model.value >= GIB && model.value % GIB === 0 ? 'GB' : 'MB');
const factor = computed(() => (unit.value === 'GB' ? GIB : MIB));
const unlimited = computed(() => model.value === -1);
const amount = computed(() => (model.value > 0 ? String(model.value / factor.value) : ''));

function setAmount(v: string | number): void {
  const n = toNumber(v);
  model.value = n > 0 ? Math.round(n * factor.value) : 0;
}
</script>

<template>
  <div class="flex items-center gap-2">
    <Input
      type="number"
      min="0"
      step="any"
      class="w-24"
      v-bind="$attrs"
      :placeholder="t('docker.edit.noLimit.placeholder')"
      :model-value="amount"
      :disabled="disabled || unlimited"
      :data-testid="testid"
      @update:model-value="setAmount"
    />
    <NativeSelect
      :aria-label="t('docker.edit.unit.label')"
      v-model="unit"
      class="w-24"
      :disabled="disabled || unlimited"
      :data-testid="`${testid}-unit`"
    >
      <option value="MB">{{ t('docker.unit.mib.option') }}</option>
      <option value="GB">{{ t('docker.unit.gib.option') }}</option>
    </NativeSelect>
    <span v-if="allowUnlimited" class="flex items-center gap-1.5 text-muted-foreground">
      <Checkbox
        :model-value="unlimited"
        :disabled="disabled"
        :data-testid="`${testid}-unlimited`"
        @update:model-value="model = $event ? -1 : 0"
        :aria-label="t('docker.edit.unlimited.label')"
      />{{ t('docker.edit.unlimited.label') }}
    </span>
  </div>
</template>
