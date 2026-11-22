<script setup lang="ts">
import { joinSentences, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import MenuIconButton from '@workbench/components/MenuIconButton.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { variableColumnMenu } from './menu';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import type { MatrixOwner } from './state/matrixModel';
import { useVariablesStore } from './state/variables';

// The ⋯ menu of one column header: the file behind the column (dotenv text, YAML), and for an
// environment its name and colour, agent access, duplicate and delete.
const props = defineProps<{ owner: MatrixOwner }>();

const dialog = useApiVariablesDialogStore();
const variablesStore = useVariablesStore();
const confirmDialogStore = useConfirmDialogStore();

async function remove(): Promise<void> {
  const confirmed = await confirmDialogStore.confirm({
    title: t('studio.api.environments.deleteConfirm.title'),
    message: joinSentences(
      t('studio.api.environments.deleteConfirm.body', { name: props.owner.name }),
      t('common.confirm.irreversible.body'),
    ),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (confirmed) await variablesStore.deleteEnvironment(props.owner.id);
}

// Environment order is the saved order of the columns: move swaps with the neighbour.
const ids = computed(() => variablesStore.environments.map((e) => e.id));
const index = computed(() => ids.value.indexOf(props.owner.id));
function move(by: -1 | 1): void {
  const next = [...ids.value];
  const to = index.value + by;
  if (index.value < 0 || to < 0 || to >= next.length) return;
  [next[index.value], next[to]] = [next[to] as string, next[index.value] as string];
  void variablesStore.reorderEnvironments(next);
}

function items() {
  return variableColumnMenu(
    {
      moveLeft: () => move(-1),
      moveRight: () => move(1),
      rename: () => dialog.open(props.owner.file, 'environment'),
      agentAccess: () => dialog.open(props.owner.file, 'agent'),
      dotenv: () => dialog.open(props.owner.file, 'variables'),
      yaml: () => dialog.open(props.owner.file, 'yaml'),
      duplicate: () => void variablesStore.duplicateEnvironment(props.owner.id),
      remove: () => void remove(),
    },
    props.owner.scope === 'environment',
    { first: index.value <= 0, last: index.value === ids.value.length - 1 },
  );
}
</script>

<template>
  <MenuIconButton
    :icon="ICON.moreActions"
    :label="t('studio.api.variables.actions.aria', { name: owner.name })"
    :items="items"
    :data-testid="`matrix-col-menu-${owner.id}`"
  />
</template>
