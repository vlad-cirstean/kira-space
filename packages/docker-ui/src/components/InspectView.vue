<script setup lang="ts">
import { t } from '@theme/i18n';
import CopyIconButton from '@workbench/components/CopyIconButton.vue';

// monaco.ts only exposes loadMonaco(), no read-only model helper — a plain <pre> keeps this a view.
defineProps<{ raw: string }>();
</script>

<template>
  <div class="relative h-full" data-testid="docker-inspect">
    <CopyIconButton
      :text="raw"
      :label="t('common.action.copyAsJson.label')"
      class="absolute right-2 top-1"
      data-testid="docker-inspect-copy"
    />
    <pre
      class="h-full overflow-auto p-3 font-data text-kira-sm"
      data-testid="docker-inspect-json"
    >{{ raw }}</pre>
  </div>
</template>
