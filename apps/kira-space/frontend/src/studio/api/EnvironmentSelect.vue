<script setup lang="ts">
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed } from 'vue';
import { useVariablesStore } from './state/variables';

const variablesStore = useVariablesStore();

// Right-aligned in the request views' `#toolbar-2` slot; stays visible with no environments.
// Popover anchors `end`: the trigger sits flush against the toolbar's right edge.
// App-drawn, not native: each row carries a colour dot.

// An optional tabId — present from both request views, so their own selection can be an
// incognito tab's own in-memory override instead of the app-wide active environment.
// environmentIdForTab/selectEnvironmentForTab already fall back to the app-wide behaviour for a
// tab that isn't incognito, and `isIncognito('')` is false for the empty-string default below (no
// tab is ever named ''), so a caller with no tab context at all needs no separate branch here — it
// gets exactly today's global behaviour for free.
const props = withDefaults(defineProps<{ tabId?: string }>(), { tabId: '' });

const activeEnvironmentId = computed(() => variablesStore.environmentIdForTab(props.tabId));
const activeEnvironment = computed(
  () => variablesStore.environments.find((e) => e.id === activeEnvironmentId.value) ?? null,
);

function selectEnvironment(id: string): void {
  void variablesStore.selectEnvironmentForTab(props.tabId, id);
}

const menuItems = computed<MenuItem[]>(() => [
  {
    type: 'radio',
    id: 'environment',
    value: activeEnvironmentId.value || null,
    items: [
      {
        type: 'option',
        id: 'environment-none',
        testId: 'api-environment-option-none',
        value: null,
        swatch: 'none',
        label: t('studio.api.environments.none.option'),
        run: () => selectEnvironment(''),
      },
      ...variablesStore.environments.map((env) => ({
        type: 'option' as const,
        id: `environment-${env.id}`,
        testId: 'api-environment-option',
        value: env.id,
        swatch: env.color,
        label: env.name,
        run: () => selectEnvironment(env.id),
      })),
    ],
  },
  { type: 'separator' },
  {
    type: 'item',
    id: 'manage',
    testId: 'api-environment-manage',
    label: t('studio.api.environments.manage.label'),
    icon: 'settings-gear',
    run: () => variablesStore.openEnvironments(),
  },
]);
</script>

<template>
  <DropdownMenu :modal="false">
    <div class="relative flex min-w-0 flex-initial ml-auto">
      <DropdownMenuTrigger as-child>
        <PickerTrigger
          size="kira"
          data-testid="api-environment-select"
          :data-value="activeEnvironmentId"
        >
          <template #leading>
            <span :class="colorMarkClass('dot', activeEnvironment?.color)" data-testid="conn-dot" />
          </template>
          {{ activeEnvironment?.name ?? t('studio.api.environments.none.option') }}
        </PickerTrigger>
      </DropdownMenuTrigger>
    </div>
    <DropdownMenuContent align="end" data-testid="api-environment-menu">
      <MenuItems :items="menuItems" />
    </DropdownMenuContent>
  </DropdownMenu>
</template>
