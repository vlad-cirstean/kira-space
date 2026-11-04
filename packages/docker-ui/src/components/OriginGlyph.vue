<script setup lang="ts">
import type { CodiconSize } from '@theme/CodiconIcon.vue';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { siKubernetes } from 'simple-icons';
import type { ContainerOrigin } from '../wire';

defineProps<{ origin: ContainerOrigin; size: CodiconSize }>();
</script>

<template>
  <CodiconIcon v-if="origin === 'compose'" name="layers" :size="size" class="text-info" />
  <svg
    v-else-if="origin === 'kubernetes' || origin === 'kind'"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    aria-hidden="true"
  >
    <path :fill="`#${siKubernetes.hex}`" :d="siKubernetes.path" />
  </svg>
  <CodiconIcon v-else-if="origin === 'swarm'" name="type-hierarchy" :size="size" />
  <CodiconIcon v-else-if="origin === 'buildx'" name="tools" :size="size" />
  <CodiconIcon v-else-if="origin === 'devcontainer'" name="remote-explorer" :size="size" />
  <CodiconIcon v-else name="beaker" :size="size" />
</template>
