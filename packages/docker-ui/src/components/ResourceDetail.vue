<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { type MessageKey, t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { displayValue, formatBytes, formatDateTime } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { nameOrShortId, stateDotTone } from '../lib/format';
import {
  useContainers,
  useImages,
  useInspect,
  useNetworks,
  useRegistryLink,
  useVolumes,
} from '../queries';
import { useDockerUiStore } from '../state/dockerUi';
import type { InspectKind } from '../wire';
import DetailSection from './DetailSection.vue';
import InspectView from './InspectView.vue';

const props = defineProps<{ kind: InspectKind; id: string }>();

const KIND_LABELS: Record<InspectKind, MessageKey> = {
  image: 'docker.field.image.label',
  volume: 'docker.resource.volume.label',
  network: 'docker.resource.network.label',
};

const ICONS: Record<InspectKind, string> = {
  image: 'package',
  volume: 'database',
  network: 'globe',
};

const ui = useDockerUiStore();
const images = useImages();
const volumes = useVolumes();
const networks = useNetworks();
const containers = useContainers();
const inspect = useInspect(
  computed(() => props.kind),
  computed(() => props.id),
);

const image = computed(() => (images.data.value ?? []).find((i) => i.id === props.id));
const volume = computed(() => (volumes.data.value ?? []).find((v) => v.name === props.id));
const network = computed(() => (networks.data.value ?? []).find((n) => n.id === props.id));

const title = computed(() => {
  if (props.kind === 'image') return nameOrShortId(image.value?.tags[0], props.id);
  if (props.kind === 'volume') return props.id;
  return nameOrShortId(network.value?.name, props.id);
});

const registry = useRegistryLink(() => (props.kind === 'image' ? props.id : ''));

const facts = computed<Array<[string, string]>>(() => {
  if (props.kind === 'image' && image.value) {
    const i = image.value;
    return [
      [t('docker.resource.tags.label'), i.tags.join(', ') || '<none>'],
      [t('docker.resource.id.label'), i.id],
      [t('docker.field.size.label'), formatBytes(i.size)],
      [t('common.created.label'), formatDateTime(i.created)],
    ];
  }
  if (props.kind === 'volume' && volume.value) {
    const v = volume.value;
    return [
      [t('docker.resource.driver.label'), v.driver],
      [t('docker.resource.mountPoint.label'), v.mountpoint],
      [t('docker.resource.scope.label'), v.scope],
      [t('common.created.label'), displayValue(v.created)],
      ...Object.entries(v.labels).map(([k, val]): [string, string] => [
        t('docker.resource.labelKey.label', { key: k }),
        val,
      ]),
    ];
  }
  if (props.kind === 'network' && network.value) {
    const n = network.value;
    return [
      [t('docker.resource.driver.label'), n.driver],
      [t('docker.resource.scope.label'), n.scope],
      [t('docker.resource.subnets.label'), displayValue(n.subnets.join(', '))],
      [
        t('docker.resource.internal.label'),
        n.internal ? t('docker.resource.yes.option') : t('docker.resource.no.option'),
      ],
    ];
  }
  return [];
});

// Image "used by" is derived from containers (image list carries only a count).
const usedBy = computed<string[]>(() => {
  if (props.kind === 'volume') return volume.value?.usedBy ?? [];
  if (props.kind === 'network') return network.value?.usedBy ?? [];
  return (containers.data.value ?? []).filter((c) => c.imageId === props.id).map((c) => c.id);
});

const usedByRows = computed(() =>
  usedBy.value.map((id) => {
    const c = (containers.data.value ?? []).find((x) => x.id === id);
    return { id, name: nameOrShortId(c?.name, id), state: c?.state };
  }),
);
</script>

<template>
  <div
    class="flex h-full flex-col overflow-auto"
    data-testid="docker-resource-detail"
    :data-kind="kind"
  >
    <div class="flex shrink-0 items-center gap-2 px-3 py-2">
      <CodiconIcon :name="ICONS[kind]" :size="16" class="text-muted-foreground" />
      <span class="min-w-0 truncate text-kira-lg font-medium" data-testid="docker-resource-title">{{
        title
      }}</span>
      <TooltipIconButton
        v-if="registry.url.value"
        icon="link-external"
        :label="t('docker.detail.openRegistry.tooltip')"
        :disabled="registry.checking.value"
        data-testid="docker-resource-registry"
        @click="registry.open()"
      />
      <span
        v-if="registry.missing.value"
        class="text-kira-sm text-muted-foreground"
        data-testid="docker-registry-missing"
        >{{
          t('docker.detail.registryMissing.status')
        }}</span
      >
      <Badge>{{ t(KIND_LABELS[kind]) }}</Badge>
    </div>
    <div class="@container p-3">
      <div class="grid grid-cols-1 items-start gap-3 @3xl:grid-cols-2">
        <DetailSection :title="t('common.details.title')">
          <KeyValueList>
            <template v-for="[ k, v ] in facts" :key="k">
              <dt>{{ k }}</dt>
              <dd class="break-all font-data">{{ v }}</dd>
            </template>
          </KeyValueList>
        </DetailSection>
        <DetailSection :title="t('docker.resource.usedBy.title')" flush>
          <p
            v-if="usedByRows.length === 0"
            class="px-3 py-2 text-muted-foreground"
            data-testid="docker-used-by-empty"
            >{{
              t('docker.resource.noContainers.emptyHint')
            }}</p
          >
          <button
            v-for="row in usedByRows"
            :key="row.id"
            type="button"
            :class="
              cn(
                rowVariants({ layout: 'menu' }),
                'h-control-lg w-full cursor-default gap-2 rounded-none px-3 text-left',
              )
            "
            data-testid="docker-used-by"
            @click="ui.select({ kind: 'container', id: row.id })"
          >
            <StatusDot :tone="row.state ? stateDotTone(row.state) : 'muted'" />
            {{ row.name }}
          </button>
        </DetailSection>
        <DetailSection :title="t('common.raw.label')" class="@3xl:col-span-2">
          <div class="h-80">
            <InspectView v-if="inspect.data.value" :raw="inspect.data.value.raw" />
          </div>
        </DetailSection>
      </div>
    </div>
  </div>
</template>
