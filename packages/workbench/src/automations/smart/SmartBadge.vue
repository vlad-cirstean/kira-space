<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { t } from '@theme/i18n';
</script>

<template>
  <TextTooltip :text="t('automations.smart.badge.tooltip')">
    <span class="inline-flex shrink-0 text-info" data-testid="script-ai-badge">
      <CodiconIcon name="sparkle" :size="13" />
    </span>
  </TextTooltip>
</template>
