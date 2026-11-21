<script setup lang="ts">
import { joinSentences, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import MenuIconButton from '@workbench/components/MenuIconButton.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
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

function items() {
  return variableColumnMenu(
    {
      rename: () => dialog.open(props.owner.file, 'environment'),
      agentAccess: () => dialog.open(props.owner.file, 'agent'),
      dotenv: () => dialog.open(props.owner.file, 'variables'),
      yaml: () => dialog.open(props.owner.file, 'yaml'),
      duplicate: () => void variablesStore.duplicateEnvironment(props.owner.id),
      remove: () => void remove(),
    },
    props.owner.scope === 'environment',
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
