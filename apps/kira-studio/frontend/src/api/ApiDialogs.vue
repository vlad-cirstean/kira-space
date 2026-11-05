<script setup lang="ts">
// The module's own front door for the host — App.vue used to import all seven of
// these dialogs and their five store bindings individually (eleven import lines); this is that
// list, once, so App.vue's own knowledge of the module shrinks to one component. Each dialog still
// guards itself on its own store's `open` flag exactly as before — this is a template-only
// reduction, not a behaviour change.
import ApiVariablesDialog from './ApiVariablesDialog.vue';
import CopyAsCurlDialog from './CopyAsCurlDialog.vue';
import DynamicValuesDialog from './DynamicValuesDialog.vue';
import EditRawRequestDialog from './EditRawRequestDialog.vue';
import ImportCurlDialog from './ImportCurlDialog.vue';
import SaveRequestDialog from './SaveRequestDialog.vue';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import { useCopyAsCurlStore, useImportCurlStore } from './state/curl';
import { useDynamicValuesStore } from './state/dynamicValues';
import { useEditRawStore } from './state/raw';
import { useSaveRequestDialogStore } from './state/saveRequestDialog';

const apiVariablesDialogStore = useApiVariablesDialogStore();
const dynamicValuesStore = useDynamicValuesStore();
const editRawStore = useEditRawStore();
const saveRequestDialogStore = useSaveRequestDialogStore();
const importCurlStore = useImportCurlStore();
const copyAsCurlStore = useCopyAsCurlStore();
</script>

<template>
  <SaveRequestDialog v-if="saveRequestDialogStore.open" />
  <ApiVariablesDialog v-if="apiVariablesDialogStore.file !== null" />
  <DynamicValuesDialog v-if="dynamicValuesStore.open" />
  <ImportCurlDialog v-if="importCurlStore.open" />
  <CopyAsCurlDialog v-if="copyAsCurlStore.open" />
  <EditRawRequestDialog v-if="editRawStore.open" />
</template>
