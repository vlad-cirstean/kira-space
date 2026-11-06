<script setup lang="ts">
import { DockerView, provideDocker } from '@kira/docker-ui';
import { studioDocker } from './context';

provideDocker(studioDocker());
</script>

<template>
  <DockerView />
</template>
