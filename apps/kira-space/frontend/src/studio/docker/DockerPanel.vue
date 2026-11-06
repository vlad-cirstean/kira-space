<script setup lang="ts">
import { DockerPanel, provideDocker } from '@kira/docker-ui';
import { studioDocker } from './context';

provideDocker(studioDocker());
</script>

<template>
  <DockerPanel />
</template>
