<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import TabStrip from '@workbench/components/TabStrip.vue';
import { computed, ref, watch } from 'vue';
import { useKubernetesUiStore } from '../state';
import { createKubeTabHost } from '../tabHost';
import { useKubeTabsStore } from '../tabs';
import ResourceTabPanel from './ResourceTabPanel.vue';

const ui = useKubernetesUiStore();
const tabs = useKubeTabsStore();
const host = createKubeTabHost(tabs);

// A tab mounts on first activation and stays until closed: a pod's log stream outlives a switch.
const activated = ref<string[]>([]);
watch(
  () => tabs.activeId,
  (id) => {
    if (id && !activated.value.includes(id)) activated.value = [...activated.value, id];
  },
  { immediate: true },
);
const mounted = computed(() => tabs.tabs.filter((tab) => activated.value.includes(tab.id)));
watch(
  () => tabs.tabs.map((tab) => tab.id),
  (ids) => {
    activated.value = activated.value.filter((id) => ids.includes(id));
  },
);
</script>

<template>
  <section
    class="flex min-h-0 min-w-80 flex-1 flex-col"
    :aria-label="t('kubernetes.tabs.aria')"
    data-testid="kube-tabs"
  >
    <div class="flex h-tabbar shrink-0 items-center border-b border-border bg-chrome">
      <TooltipIconButton
        class="ml-1 shrink-0"
        :icon="ui.listHidden ? 'layout-sidebar-left-off' : 'layout-sidebar-left'"
        :label="
          ui.listHidden ? t('kubernetes.tabs.showList.label') : t('kubernetes.tabs.hideList.label')
        "
        :pressed="ui.listHidden"
        data-testid="kube-list-toggle"
        @click="ui.listHidden = !ui.listHidden"
      />
      <TabStrip :host="host" class="min-w-0 flex-1" />
    </div>
    <div class="min-h-0 flex-1">
      <ResourceTabPanel v-for="tab in mounted" v-show="tab.active" :key="tab.id" :tab="tab" />
    </div>
  </section>
</template>
