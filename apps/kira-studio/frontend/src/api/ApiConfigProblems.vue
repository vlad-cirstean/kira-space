<script setup lang="ts">
import { t } from '@theme/i18n';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { computed } from 'vue';
import { control } from '../bridge/control';
import { useApiConfig } from './state/apiConfig';

// One bar per config file that fails to parse or validate. The rows keep following the file's last
// valid text meanwhile; the bar shows the first problem with its line.
const config = useApiConfig();
const broken = computed(() => (config.data.value?.files ?? []).filter((f) => f.problem !== null));
</script>

<template>
  <section
    v-if="broken.length > 0"
    class="flex shrink-0 flex-col gap-2 p-2"
    :aria-label="t('studio.api.config.problems.aria')"
    data-testid="api-config-problems"
  >
    <div v-for="file in broken" :key="file.file" class="flex flex-col gap-1">
      <ConfigFileBar
        :path="file.path"
        :error="file.problem"
        :open="() => control.configFilesOpen({ folder: 'api', file: file.file })"
        :reveal="() => control.configFilesReveal({ folder: 'api', file: file.file })"
        :about="() => control.configFilesOpen({ folder: 'api', file: 'AGENTS.md' })"
      />
      <span
        v-if="file.lastGood"
        class="text-kira-sm text-subtle"
        data-testid="api-config-last-good"
      >
        {{ t('studio.api.config.lastGood.hint') }}
      </span>
    </div>
  </section>
</template>
