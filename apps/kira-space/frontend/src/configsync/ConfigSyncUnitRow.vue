<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';

import { type MessageKey, t } from '@theme/i18n';
import { computed, useId } from 'vue';
import type { ConfigSyncChoice, ConfigSyncKind, ConfigSyncRow } from './domain';
import { unitKey } from './domain';

const props = defineProps<{
  row: ConfigSyncRow;
  selected: boolean;
  choice: ConfigSyncChoice;
}>();
const emit = defineEmits<{
  select: [];
  'update:choice': [choice: ConfigSyncChoice];
  deactivate: [off: boolean];
}>();

const KIND_LABEL: Record<ConfigSyncKind, MessageKey> = {
  add: 'common.action.add.label',
  modify: 'space.configSync.kind.modify.label',
  delete: 'common.action.delete.label',
  conflict: 'space.ade.board.branchTag.conflict.label',
  blocked: 'space.common.status.blocked.status',
  unchanged: 'studio.http.diff.unchangedRow.status',
};
const KIND_TONE: Record<ConfigSyncKind, 'ok' | 'info' | 'warn' | 'error' | 'default'> = {
  add: 'ok',
  modify: 'info',
  delete: 'warn',
  conflict: 'warn',
  blocked: 'error',
  unchanged: 'default',
};

const switchId = useId();
const label = computed(() => unitKey(props.row));
</script>

<template>
  <li
    class="flex flex-col gap-1 border-b border-border px-3 py-2 text-kira-md"
    :class="selected ? 'bg-field' : 'hover:bg-field/50'"
    :data-testid="`config-sync-row-${row.id}`"
  >
    <button
      type="button"
      class="flex w-full cursor-default flex-col gap-1 text-left"
      :aria-pressed="selected"
      :data-testid="`config-sync-select-${row.id}`"
      @click="emit('select')"
    >
      <span class="flex w-full items-center gap-2">
        <span class="min-w-0 flex-1 truncate font-data">{{ label }}</span>
        <Badge v-if="row.runsCommands" variant="warn" data-testid="config-sync-row-runs">
          {{ t('space.configSync.row.runsCommands.label') }}
        </Badge>
        <Badge :variant="KIND_TONE[row.kind]" :data-testid="`config-sync-row-kind-${row.id}`">
          {{ t(KIND_LABEL[row.kind]) }}
        </Badge>
      </span>
      <span
        v-if="row.problem"
        class="text-kira-sm text-error"
        :data-testid="`config-sync-row-problem-${row.id}`"
      >
        {{ row.problem }}
      </span>
      <span v-if="row.unsetSecrets.length > 0" class="text-kira-sm text-muted-foreground">
        {{ t('space.configSync.row.unsetSecrets.hint', { names: row.unsetSecrets.join(', ') }) }}
      </span>
    </button>
    <div class="flex items-center gap-3">
      <NativeSelect
        v-if="row.kind === 'conflict'"
        variant="bordered"
        :model-value="choice"
        :data-testid="`config-sync-choice-${row.id}`"
        @update:model-value="(v) => emit('update:choice', v === 'shared' ? 'shared' : 'mine')"
      >
        <option value="mine">{{ t('space.configSync.row.mine.option') }}</option>
        <option value="shared">{{ t('space.configSync.row.shared.option') }}</option>
      </NativeSelect>
      <span class="ml-auto flex items-center gap-1.5">
        <Label :for="switchId" class="text-kira-sm text-muted-foreground">
          {{ t('space.configSync.row.deactivate.label') }}
        </Label>
        <Checkbox
          :id="switchId"
          :model-value="row.deactivated"
          :data-testid="`config-sync-deactivate-${row.id}`"
          @update:model-value="(v) => emit('deactivate', v === true)"
        />
      </span>
    </div>
  </li>
</template>
