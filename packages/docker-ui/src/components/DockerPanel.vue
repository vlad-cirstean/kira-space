<script setup lang="ts">
import { useQueryClient } from '@tanstack/vue-query';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { type MessageKey, t } from '@theme/i18n';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed, ref, useTemplateRef } from 'vue';
import {
  useContainers,
  useDockerLiveSync,
  useDockerStatus,
  useImages,
  useNetworks,
  useVolumes,
} from '../queries';
import { type DockerSection, useDockerUiStore } from '../state/dockerUi';
import { useStatsSubscription } from '../state/useStatsSubscription';
import ConnectionState from './ConnectionState.vue';
import ContainerList from './ContainerList.vue';
import EndpointChip from './EndpointChip.vue';
import ImageList from './ImageList.vue';
import NetworkList from './NetworkList.vue';
import RestoreBanner from './RestoreBanner.vue';
import VolumeList from './VolumeList.vue';

const ui = useDockerUiStore();
const qc = useQueryClient();
const status = useDockerStatus();
const containers = useContainers();
const images = useImages();
const volumes = useVolumes();
const networks = useNetworks();

useDockerLiveSync();

const runningIds = computed(() =>
  (containers.data.value ?? []).filter((c) => c.state === 'running').map((c) => c.id),
);
useStatsSubscription(runningIds);

const ok = computed(() => status.data.value?.state === 'ok');

const sections = computed<{ id: DockerSection; label: MessageKey; count: number }[]>(() => [
  {
    id: 'containers',
    label: 'docker.section.containers.label',
    count: containers.data.value?.length ?? 0,
  },
  { id: 'images', label: 'docker.section.images.label', count: images.data.value?.length ?? 0 },
  { id: 'volumes', label: 'docker.section.volumes.label', count: volumes.data.value?.length ?? 0 },
  { id: 'networks', label: 'docker.field.networks.label', count: networks.data.value?.length ?? 0 },
]);

const sectionItems = computed(() =>
  sections.value.map((x) => ({
    value: x.id,
    label: t(x.label),
    count: x.count,
    testid: `docker-section-${x.id}`,
  })),
);

const running = computed(
  () => (containers.data.value ?? []).filter((c) => c.state === 'running').length,
);
const stopped = computed(() => (containers.data.value?.length ?? 0) - running.value);

const rootEl = useTemplateRef<HTMLElement>('rootEl');
const { showSearch, toggleSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => ok.value,
  getSearch: () => ui.search,
  setSearch: (v) => {
    ui.search = v;
  },
});

const refreshing = ref(false);
async function refresh(): Promise<void> {
  refreshing.value = true;
  try {
    await qc.invalidateQueries({ queryKey: ['docker'] });
  } finally {
    refreshing.value = false;
  }
}
</script>

<template>
  <div ref="rootEl" class="flex h-full flex-col" data-testid="docker-panel">
    <PanelHeader>
      {{ t('common.product.docker.label') }}
      <EndpointChip />
      <template #actions>
        <TooltipIconButton
          icon="search"
          :label="
            showSearch ? t('common.action.hideSearch.label') : t('common.action.search.label')
          "
          :pressed="showSearch"
          :disabled="!ok"
          data-testid="toggle-search"
          @click="toggleSearch"
        />
        <TooltipIconButton
          icon="dashboard"
          :label="t('docker.panel.engineOverview.tooltip')"
          :disabled="!ok || !ui.selection"
          data-testid="docker-overview-home"
          @click="ui.select(null)"
        />
        <TooltipIconButton
          icon="refresh"
          :label="t('common.action.refresh.label')"
          data-testid="docker-refresh"
          :disabled="refreshing"
          @click="refresh"
        />
      </template>
    </PanelHeader>
    <PanelBar v-if="ok && showSearch">
      <SearchField v-model="ui.search" data-testid="tree-search" />
    </PanelBar>
    <template v-if="ok">
      <RestoreBanner />
      <PanelBar>
        <SecondaryTabs
          :model-value="ui.section"
          :items="sectionItems"
          :aria-label="t('docker.panel.sections.aria')"
          @update:model-value="(v) => ui.setSection(v as DockerSection)"
        >
          <template #item="{ item }">
            {{ item.label }}
            <span class="text-kira-sm text-muted-foreground" data-testid="docker-section-count">{{
              item.count
            }}</span>
          </template>
        </SecondaryTabs>
      </PanelBar>
      <div
        v-if="ui.section === 'containers'"
        class="flex h-control shrink-0 cursor-default items-center gap-1.5 px-1.5 text-kira-sm text-muted-foreground"
      >
        <Checkbox
          id="docker-show-stopped"
          v-model="ui.showStopped"
          data-testid="docker-show-stopped"
        />
        <Label for="docker-show-stopped" class="cursor-default">{{
          t('docker.panel.showStopped.label')
        }}</Label>
        <span class="ml-auto" data-testid="docker-summary">{{
          stopped
            ? t('docker.panel.summaryStopped.label', { running, stopped })
            : t('common.state.runningCount.status', { count: running })
        }}</span>
      </div>
      <ContainerList v-if="ui.section === 'containers'" />
      <ImageList v-else-if="ui.section === 'images'" />
      <VolumeList v-else-if="ui.section === 'volumes'" />
      <NetworkList v-else />
    </template>
    <ConnectionState v-else />
  </div>
</template>
