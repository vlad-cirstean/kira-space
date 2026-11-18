<script setup lang="ts">
import StatusDot from '@theme/components/StatusDot.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { t } from '@theme/i18n';

defineProps<{ tip: string; testid: string }>();
</script>

<template>
  <TextTooltip :text="tip">
    <StatusDot tone="info" :label="t('space.ade.updates.dot.aria')" :data-testid="testid" />
  </TextTooltip>
</template>
