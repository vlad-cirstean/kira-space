<script setup lang="ts">
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import TabStripNewButton from '@workbench/components/TabStripNewButton.vue';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { openApiRequestTab, openGrpcRequestTab } from './tabs';

// The API tab strip's "+": a dropdown under the button, wording shared with the collection menu.
const contextMenuStore = useContextMenuStore();

function menuItems(): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'new-request',
      label: t('studio.api.common.newRequest.label'),
      icon: ICON.add,
      run: () => {
        openApiRequestTab();
      },
    },
    {
      type: 'item',
      id: 'new-grpc-request',
      label: t('shared.grpc.newRequest.title'),
      icon: 'symbol-interface',
      run: () => {
        openGrpcRequestTab();
      },
    },
  ];
}

function onClick(button: HTMLButtonElement): void {
  const rect = button.getBoundingClientRect();
  contextMenuStore.openContextMenuAt(rect.left, rect.bottom + 2, menuItems());
}
</script>

<template>
  <TabStripNewButton
    :label="t('workbench.tabStrip.newTab.label')"
    :tooltip="t('workbench.tabStrip.newTab.label')"
    has-popup
    @click="onClick"
  />
</template>
