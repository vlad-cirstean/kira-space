<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
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

// reka radio values cannot be '' — "No environment" travels as this sentinel.
const NONE = '__none__';
const radioValue = computed(() =>
  activeEnvironmentId.value === '' ? NONE : activeEnvironmentId.value,
);

const activeEnvironmentId = computed(() => variablesStore.environmentIdForTab(props.tabId));
const activeEnvironment = computed(
  () => variablesStore.environments.find((e) => e.id === activeEnvironmentId.value) ?? null,
);

function onSelect(value: unknown): void {
  const id = String(value);
  void variablesStore.selectEnvironmentForTab(props.tabId, id === NONE ? '' : id);
}

function manage(): void {
  variablesStore.openEnvironments();
}
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
      <DropdownMenuRadioGroup :model-value="radioValue" @update:model-value="onSelect">
        <DropdownMenuRadioItem
          :value="NONE"
          data-testid="api-environment-option-none"
          data-value=""
        >
          <span :class="colorMarkClass('dot', 'none')" data-testid="conn-dot" />
          <span class="flex-1 truncate">{{ t('studio.api.environments.none.option') }}</span>
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
        <DropdownMenuRadioItem
          v-for="env in variablesStore.environments"
          :key="env.id"
          :value="env.id"
          data-testid="api-environment-option"
          :data-value="env.id"
        >
          <span :class="colorMarkClass('dot', env.color)" data-testid="conn-dot" />
          <span class="flex-1 truncate">{{ env.name }}</span>
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
      <DropdownMenuSeparator />
      <DropdownMenuItem data-testid="api-environment-manage" @select="manage">
        <span class="flex-1 truncate">{{ t('studio.api.environments.manage.label') }}</span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
