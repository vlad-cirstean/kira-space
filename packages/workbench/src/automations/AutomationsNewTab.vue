<script setup lang="ts">
import { t } from '@theme/i18n';
import TabStripNewButton from '@workbench/components/TabStripNewButton.vue';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { useAutomationsModule } from './module';

// The tab strip's own "+" for the Automations module — one plain, unscoped session at the
// resolved home directory. TabStripNewButton carries the wrapper/Tooltip/button markup both apps'
// own "+" already shared; this file keeps exactly the automations module's own menu content,
// verbatim from Kira Studio's own WorkbenchShell.vue.
const ctx = useAutomationsModule();
const contextMenuStore = useContextMenuStore();

function menuItems(): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'new-terminal',
      label: t('common.module.terminal.label'),
      icon: 'terminal-bash',
      disabled: ctx.defaultCwd() === '',
      run: () => {
        ctx.openTerminalTab({ cwd: ctx.defaultCwd() });
      },
    },
  ];
}

// P83 §9: the tab strip's own "+" — a dropdown anchored under the button, not the click point.
function onClick(button: HTMLButtonElement): void {
  const rect = button.getBoundingClientRect();
  contextMenuStore.openContextMenuAt(rect.left, rect.bottom + 2, menuItems());
}
</script>

<template>
  <TabStripNewButton
    :label="t('common.tab.new.label')"
    :tooltip="t('common.tab.new.label')"
    has-popup
    @click="onClick"
  />
</template>
