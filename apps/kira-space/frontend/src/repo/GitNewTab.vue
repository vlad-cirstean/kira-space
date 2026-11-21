<script setup lang="ts">
import { t } from '@theme/i18n';
import TabStripNewButton from '@workbench/components/TabStripNewButton.vue';
import { computed } from 'vue';
import { openActiveRepoTerminal } from '../state/repoTabs';
import { GENERAL_WORKSPACE, useWorkspaceStore } from '../state/workspace';

// P128 §2.6: the Git module's own tab-strip "+" — a terminal at the active repository's own root,
// moved verbatim (label/tooltip/click target) off WorkbenchShell.vue onto the shared
// TabStripNewButton (P128 §2.3), which both apps' own "+" already used identical markup for.
const workspaceStore = useWorkspaceStore();

const visible = computed(() => workspaceStore.active !== GENERAL_WORKSPACE);

function onClick(): void {
  openActiveRepoTerminal();
}
</script>

<template>
  <TabStripNewButton
    v-if="visible"
    :label="t('common.newTerminal.label')"
    :tooltip="t('space.repo.newTerminal.tooltip')"
    @click="onClick"
  />
</template>
