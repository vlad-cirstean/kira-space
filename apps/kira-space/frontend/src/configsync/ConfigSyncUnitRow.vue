<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Badge } from '@theme/components/ui/badge';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';

import { type MessageKey, t } from '@theme/i18n';
import { CHANGE_TONE, type StatusTone, TONE_BADGE } from '@theme/lib/statusTone';
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

const CHOICE_ITEMS = computed(() => [
  {
    value: 'mine',
    label: t('space.configSync.row.mine.option'),
    testid: `config-sync-choice-${props.row.id}-mine`,
  },
  {
    value: 'shared',
    label: t('space.configSync.row.shared.option'),
    testid: `config-sync-choice-${props.row.id}-shared`,
  },
]);

const KIND_LABEL: Record<ConfigSyncKind, MessageKey> = {
  add: 'common.action.add.label',
  modify: 'space.configSync.kind.modify.label',
  delete: 'common.action.delete.label',
  conflict: 'space.ade.board.branchTag.conflict.label',
  blocked: 'space.common.status.blocked.status',
  unchanged: 'common.diff.unchangedRow.status',
  unmatched: 'space.configSync.kind.unmatched.label',
};
const KIND_TONE: Record<ConfigSyncKind, StatusTone> = {
  add: CHANGE_TONE.added,
  modify: CHANGE_TONE.changed,
  delete: CHANGE_TONE.removed,
  conflict: 'error',
  blocked: 'error',
  unchanged: CHANGE_TONE.unchanged,
  unmatched: 'neutral',
};

const switchId = useId();
const label = computed(() => unitKey(props.row));
// A repos unit shows its repository, or the origin when no repository matched.
const repoLine = computed(() => props.row.repo || props.row.origin || '');
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
        <Badge
          :variant="TONE_BADGE[KIND_TONE[row.kind]]"
          :data-testid="`config-sync-row-kind-${row.id}`"
        >
          {{ t(KIND_LABEL[row.kind]) }}
        </Badge>
      </span>
      <span
        v-if="repoLine"
        class="truncate font-data text-kira-sm text-muted-foreground"
        :data-testid="`config-sync-row-repo-${row.id}`"
      >
        {{ repoLine }}
      </span>
      <FieldError v-if="row.problem" :data-testid="`config-sync-row-problem-${row.id}`">
        {{ row.problem }}
      </FieldError>
      <span v-if="row.unsetSecrets.length > 0" class="text-kira-sm text-muted-foreground">
        {{ t('space.configSync.row.unsetSecrets.hint', { names: row.unsetSecrets.join(', ') }) }}
      </span>
    </button>
    <div class="flex items-center gap-3">
      <SecondaryTabs
        v-if="row.kind === 'conflict'"
        variant="choice"
        :aria-label="t('space.configSync.row.choice.label')"
        :model-value="choice"
        :items="CHOICE_ITEMS"
        :data-testid="`config-sync-choice-${row.id}`"
        @update:model-value="(v) => emit('update:choice', v === 'shared' ? 'shared' : 'mine')"
      />
      <span v-if="row.kind !== 'unmatched'" class="ml-auto flex items-center gap-1.5">
        <Checkbox
          :id="switchId"
          :model-value="row.deactivated"
          :data-testid="`config-sync-deactivate-${row.id}`"
          @update:model-value="(v) => emit('deactivate', v === true)"
        />
        <Label :for="switchId" class="text-kira-sm text-muted-foreground">
          {{ t('space.configSync.row.deactivate.label') }}
        </Label>
      </span>
    </div>
  </li>
</template>
