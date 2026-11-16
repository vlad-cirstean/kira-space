<script setup lang="ts">
import { Alert, AlertAction, AlertDescription, AlertTitle } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import ConfigFileProblem from '@workbench/config/ConfigFileProblem.vue';
import { configName } from '@workbench/config/configName';
import { computed } from 'vue';
import { useApiConfig } from './state/apiConfig';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';

// One bar per config file that fails to parse or validate. The rows keep following the file's last
// valid text meanwhile; the bar shows the first problem with its line. A collection that loaded
// without some items gets a warning listing each skipped item.
const config = useApiConfig();
const variablesDialogStore = useApiVariablesDialogStore();
const files = computed(() => config.data.value?.files ?? []);
const broken = computed(() => files.value.filter((f) => f.problem !== null));
const partial = computed(() =>
  files.value.filter((f) => f.problem === null && f.skipped.length > 0),
);
</script>

<template>
  <section
    v-if="broken.length + partial.length > 0"
    class="flex shrink-0 flex-col gap-2 p-2"
    :aria-label="t('studio.api.config.problems.aria')"
    data-testid="api-config-problems"
  >
    <div v-for="file in broken" :key="file.file" class="flex flex-col gap-1">
      <div class="flex min-w-0 items-center gap-2">
        <span class="shrink-0 text-kira-sm font-medium" data-testid="api-config-problem-name">{{
          file.name || configName(file.file)
        }}</span>
        <ConfigFileProblem :error="file.problem" class="min-w-0 flex-1" />
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="api-config-problem-edit"
          @click="variablesDialogStore.open(file.file, 'yaml')"
        >
          {{ t('common.action.edit.label') }}
        </Button>
      </div>
      <span
        v-if="file.lastGood"
        class="text-kira-sm text-subtle"
        data-testid="api-config-last-good"
      >
        {{ t('studio.api.config.lastGood.hint') }}
      </span>
    </div>
    <Alert v-for="file in partial" :key="file.file" variant="warn" data-testid="api-config-skipped">
      <AlertTitle>
        {{
          t('studio.api.config.skipped.title', {
            count: file.skipped.length,
            name: file.name || configName(file.file),
          })
        }}
      </AlertTitle>
      <AlertDescription>
        <p>{{ t('studio.api.config.skipped.hint') }}</p>
        <ul class="mt-1 flex flex-col gap-0.5">
          <li v-for="item in file.skipped" :key="item.line" data-testid="api-config-skipped-item">
            {{
              t('studio.api.config.skipped.label', {
                line: String(item.line),
                label: item.name || item.id || `#${item.index + 1}`,
                message: item.message,
              })
            }}
          </li>
        </ul>
      </AlertDescription>
      <AlertAction>
        <Button
          variant="link"
          size="inline"
          data-testid="api-config-skipped-edit"
          @click="variablesDialogStore.open(file.file, 'yaml')"
        >
          {{ t('common.action.edit.label') }}
        </Button>
      </AlertAction>
    </Alert>
  </section>
</template>
