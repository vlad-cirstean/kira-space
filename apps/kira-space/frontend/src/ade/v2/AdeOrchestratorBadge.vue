<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import AdeTip from './AdeTip.vue';
import { TONE_TAG_CLASS } from './tones';

// Marks an orchestrator task on its card, panel header and archived panel.
defineProps<{ iconOnly?: boolean }>();
</script>

<template>
  <AdeTip :text="t('space.ade.orchestrator.badge.tooltip')">
    <Badge
      variant="chip"
      :class="TONE_TAG_CLASS.blue"
      :aria-label="t('space.ade.orchestrator.badge.label')"
      data-testid="ade-orchestrator-badge"
    >
      <CodiconIcon name="organization" :size="12" />
      <template v-if="!iconOnly">{{ t('space.ade.orchestrator.badge.label') }}</template>
    </Badge>
  </AdeTip>
</template>
