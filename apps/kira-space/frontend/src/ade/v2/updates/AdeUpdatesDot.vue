<script setup lang="ts">
import StatusDot from '@theme/components/StatusDot.vue';
import { t } from '@theme/i18n';
import AdeTip from '../AdeTip.vue';

defineProps<{ tip: string; testid: string }>();
</script>

<template>
  <AdeTip :text="tip">
    <StatusDot tone="info" :label="t('space.ade.updates.dot.aria')" :data-testid="testid" />
  </AdeTip>
</template>
