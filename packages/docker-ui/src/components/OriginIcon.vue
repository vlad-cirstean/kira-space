<script setup lang="ts">
import type { CodiconSize } from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { ContainerOrigin } from '../wire';
import OriginGlyph from './OriginGlyph.vue';

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    origin: ContainerOrigin;
    name?: string;
    size?: CodiconSize;
    /** `attribute`: label goes in `data-kira-tip` for a hoisted `AttributeTooltip`. */
    tip?: 'tooltip' | 'attribute';
  }>(),
  {
    name: '',
    size: 13,
    tip: 'tooltip',
  },
);

const label = computed(() => {
  const n = props.name;
  switch (props.origin) {
    case 'compose':
      return t('docker.origin.compose.tooltip', { tool: 'Compose' });
    case 'devcontainer':
      return n
        ? t('docker.origin.devcontainerNamed.tooltip', { name: n })
        : t('docker.origin.devcontainer.tooltip');
    case 'testcontainers':
      return 'Testcontainers';
    case 'kind':
      return t('docker.origin.kind.tooltip', { name: n });
    case 'kubernetes':
      return t('docker.origin.kubernetes.tooltip', { tool: 'Kubernetes', name: n });
    case 'swarm':
      return t('docker.origin.swarm.tooltip', { tool: 'Swarm', name: n });
    case 'buildx':
      return t('docker.origin.buildx.tooltip', { tool: 'Buildx', name: n });
    default:
      return '';
  }
});
</script>

<template>
  <template v-if="origin !== ''">
    <span
      v-if="tip === 'attribute'"
      class="inline-flex shrink-0 items-center text-muted-foreground"
      data-testid="docker-origin-icon"
      :data-origin="origin"
      :aria-label="label"
      :data-kira-tip="label"
      v-bind="$attrs"
    >
      <OriginGlyph :origin="origin" :size="size" />
    </span>
    <TextTooltip v-else :text="label"
      ><span
        class="inline-flex shrink-0 items-center text-muted-foreground"
        data-testid="docker-origin-icon"
        :data-origin="origin"
        :aria-label="label"
        v-bind="$attrs"
      >
        <OriginGlyph :origin="origin" :size="size" />
      </span></TextTooltip
    >
  </template>
</template>
