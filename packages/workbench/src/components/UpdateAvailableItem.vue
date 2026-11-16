<script setup lang="ts">
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import { t } from '@theme/i18n';
import StatusBarItem from '@workbench/components/StatusBarItem.vue';

// Hoisted from Kira Studio's own StatusBar.vue (same data-testid, same markup) —
// AppMetricsItem.vue's own plain-props shape, shared with Kira Space's own status bar. The click
// used to open GitHub's release page directly (OpenReleasePage); it now tells the store to
// open the in-app dialog (the deliberate behavior change).
defineProps<{
  targetVersion: string;
  newestVersion: string;
  currentVersion: string;
  stepped: boolean;
}>();

const emit = defineEmits<(e: 'open') => void>();
</script>

<template>
  <StatusBarItem
    icon="cloud-download"
    tone="info"
    data-testid="update-available"
    @click="emit('open')"
  >
    {{ t('workbench.update.available.label', { version: targetVersion }) }}
    <template #tip>
      <TooltipDetails
        :title="t('workbench.update.available.tooltip')"
        :rows="[
          { label: t('workbench.update.newVersion.label'), value: targetVersion },
          ...(stepped ? [{ label: t('workbench.update.newest.label'), value: newestVersion }] : []),
          { label: t('workbench.update.installed.label'), value: currentVersion },
        ]"
        :hint="t('workbench.update.available.hint')"
      />
    </template>
  </StatusBarItem>
</template>
