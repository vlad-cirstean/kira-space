<script setup lang="ts" generic="M extends string">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import type { KeepAwakeStatus } from '../bridge/createCoreControl';
import type { ModeRegistry } from '../modes';
import ModeSwitcher from './ModeSwitcher.vue';
import TitleBarBase from './TitleBar.vue';
import TitleBarWindowActions from './TitleBarWindowActions.vue';

// Both apps' title bar: app title, mode switcher, the two panel toggles, Settings, window actions. Stores and
// bridge calls stay app-side (each app's own instances), so state arrives as props and actions
// leave as events. `panelLabel` names the left panel's toggle (Connections / Repositories).
// `beforeModeChange` can veto a switch (Space's unsaved ADE workflow edits).
const props = defineProps<{
  order: readonly M[];
  modes: ModeRegistry<M>;
  active: M;
  appTitle: string;
  panelLabel: string;
  projectVisible: boolean;
  operationsVisible: boolean;
  keepAwake: KeepAwakeStatus;
  beforeModeChange?: (mode: M) => boolean | Promise<boolean>;
}>();

const emit = defineEmits<{
  select: [mode: M];
  toggleProject: [];
  toggleOperations: [];
  openSettings: [];
  toggleKeepAwake: [];
  newWindow: [];
}>();

async function onSelect(mode: M): Promise<void> {
  if (props.beforeModeChange && !(await props.beforeModeChange(mode))) return;
  emit('select', mode);
}
</script>

<template>
  <TitleBarBase>
    <!-- First in-flow child: sits at the traffic-light inset. Plain text stays in the drag region. -->
    <span
      class="shrink-0 whitespace-nowrap text-kira-md font-medium"
      data-testid="titlebar-app-title"
      >{{
        appTitle
      }}</span
    >
    <ModeSwitcher :order="order" :modes="modes" :active="active" @select="onSelect" />

    <!-- Every interactive child overrides --wails-draggable (wails-no-drag), or clicking it would
         also start a window drag (drag.ts reads the event target's computed style). Filled icon
         when a panel is visible, the codicon "-off" companion when it is not. -->
    <div class="flex items-center gap-0.5 ml-auto wails-no-drag">
      <TooltipIconButton
        :icon="projectVisible ? 'layout-sidebar-left' : 'layout-sidebar-left-off'"
        :label="panelLabel"
        :icon-size="16"
        variant="title"
        size="title"
        :aria-pressed="projectVisible"
        data-testid="toggle-project-panel"
        @click="emit('toggleProject')"
      />
      <TooltipIconButton
        :icon="operationsVisible ? 'layout-panel' : 'layout-panel-off'"
        :label="t('common.module.operations.label')"
        :icon-size="16"
        variant="title"
        size="title"
        :aria-pressed="operationsVisible"
        data-testid="toggle-operations-panel"
        @click="emit('toggleOperations')"
      />
      <TooltipIconButton
        :icon="ICON.settings"
        :label="t('common.module.settings.label')"
        :icon-size="16"
        variant="title"
        size="title"
        data-testid="open-settings"
        @click="emit('openSettings')"
      />
      <TitleBarWindowActions
        :keep-awake="keepAwake"
        @toggle-keep-awake="emit('toggleKeepAwake')"
        @new-window="emit('newWindow')"
      />
    </div>

    <template #settings>
      <slot name="settings" />
    </template>
  </TitleBarBase>
</template>
