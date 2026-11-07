<script setup lang="ts">
import { shortSha } from '@kira/git-core';
import type { EditorLanguageId } from '@shared/domain/editor';
import EmptyState from '@theme/components/EmptyState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Alert } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { type MessageKey, t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import Identifier from '@workbench/components/Identifier.vue';
import TitleBarBase from '@workbench/components/TitleBar.vue';
import MonacoDiff from '@workbench/editor/MonacoDiff.vue';
import { computed, reactive, ref } from 'vue';
import { control } from '../bridge/control';
import ConfigSyncUnitRow from './ConfigSyncUnitRow.vue';
import type { ConfigSyncChoice, ConfigSyncRow } from './domain';
import { unitKey } from './domain';
import { useConfigSyncMutations, useConfigSyncPreview } from './queries';

// Preview of the fetched commit. Pull sends the conflict choices with the single-use token.
const preview = useConfigSyncPreview(() => true);
const { apply, setDeactivated } = useConfigSyncMutations();

const MODULE_LABEL: Record<string, MessageKey> = {
  connections: 'common.module.connections.label',
  api: 'space.settings.configSync.module.api.label',
  runs: 'space.settings.configSync.module.runs.label',
  workflows: 'space.settings.configSync.module.workflows.label',
  steps: 'space.settings.configSync.module.steps.label',
  agent: 'space.settings.configSync.module.agent.label',
  prompts: 'space.settings.configSync.module.prompts.label',
};

const choices = reactive<Record<string, ConfigSyncChoice>>({});
const selectedId = ref('');

const rows = computed<ConfigSyncRow[]>(() =>
  (preview.data.value?.rows ?? []).filter((r) => r.kind !== 'unchanged' || r.deactivated),
);
const groups = computed(() => {
  const out: { module: string; rows: ConfigSyncRow[] }[] = [];
  for (const row of rows.value) {
    let g = out.find((x) => x.module === row.module);
    if (!g) {
      g = { module: row.module, rows: [] };
      out.push(g);
    }
    g.rows.push(row);
  }
  return out;
});
const selected = computed(() => rows.value.find((r) => r.id === selectedId.value) ?? null);
const diff = computed(() => {
  const row = selected.value;
  if (!row) return null;
  return (
    preview.data.value?.files.find((f) => f.module === row.module && f.file === row.file) ?? null
  );
});
const language = computed<EditorLanguageId>(() => {
  const file = selected.value?.file ?? '';
  if (file.endsWith('.md')) return 'markdown';
  if (file.endsWith('.json')) return 'json';
  return 'yaml';
});
const failure = computed(() => preview.error.value ?? apply.error.value);

const choiceOf = (row: ConfigSyncRow): ConfigSyncChoice => choices[row.id] ?? 'mine';

function onDeactivate(row: ConfigSyncRow, off: boolean): void {
  setDeactivated.mutate({ module: row.module, unit: unitKey(row), off });
}

async function onPull(): Promise<void> {
  const data = preview.data.value;
  if (!data) return;
  const picked: Record<string, ConfigSyncChoice> = {};
  for (const row of rows.value) if (row.kind === 'conflict') picked[row.id] = choiceOf(row);
  await apply.mutateAsync({ token: data.token, choices: picked });
  await control.configSyncCloseReview();
}

function onCancel(): void {
  fireAndForget(control.configSyncCloseReview(), 'configSync');
}
</script>

<template>
  <div class="flex h-full flex-col" data-testid="config-sync-window">
    <TitleBarBase>
      <span class="min-w-0 truncate text-kira-md font-medium" data-testid="config-sync-title">{{
        t('space.configSync.window.title')
      }}</span>
    </TitleBarBase>
    <div v-if="failure" class="mx-3 mt-2">
      <Alert variant="error" data-testid="config-sync-error">{{ errorText(failure) }}</Alert>
    </div>
    <div class="flex min-h-0 flex-1 bg-chrome px-1.5 pb-0.5">
      <section class="flex min-h-0 min-w-0 flex-1 overflow-hidden rounded-kira bg-bg">
        <div v-if="preview.isPending.value" class="m-auto text-muted-foreground">
          {{ t('space.configSync.window.loading.loading') }}
        </div>
        <EmptyState
          v-else-if="rows.length === 0"
          fill
          icon="check"
          :title="t('space.configSync.window.empty.empty')"
          data-testid="config-sync-empty"
        />
        <template v-else>
          <div class="flex w-96 shrink-0 flex-col overflow-y-auto border-r border-border">
            <template v-for="g in groups" :key="g.module">
              <SectionHeading
                class="sticky top-0 bg-chrome"
                :label="MODULE_LABEL[g.module] ? t(MODULE_LABEL[g.module]) : g.module"
                :count="g.rows.length"
              />
              <ul>
                <ConfigSyncUnitRow
                  v-for="row in g.rows"
                  :key="row.id"
                  :row="row"
                  :selected="row.id === selectedId"
                  :choice="choiceOf(row)"
                  @select="selectedId = row.id"
                  @update:choice="(c) => (choices[row.id] = c)"
                  @deactivate="(off) => onDeactivate(row, off)"
                />
              </ul>
            </template>
          </div>
          <div class="min-h-0 min-w-0 flex-1">
            <MonacoDiff
              v-if="diff"
              :key="`${diff.module}/${diff.file}`"
              class="h-full"
              :original="diff.current"
              :modified="diff.after"
              :language="language"
              data-testid="config-sync-diff"
            />
            <EmptyState
              v-else
              fill
              icon="file"
              :title="t('space.configSync.diff.empty.empty')"
              data-testid="config-sync-diff-empty"
            />
          </div>
        </template>
      </section>
    </div>
    <footer class="flex items-center gap-2 border-t border-border px-3 py-2">
      <span class="font-data text-kira-sm text-muted-foreground" data-testid="config-sync-commit">
        {{ t('space.configSync.footer.commit.label') }}
        <Identifier :value="preview.data.value?.commit ?? ''" :short="shortSha" />
      </span>
      <span class="ml-auto flex gap-2">
        <Button variant="dialog" size="kira-lg" data-testid="config-sync-cancel" @click="onCancel">
          {{ t('common.action.cancel.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!preview.data.value"
          :pending="apply.isPending.value"
          data-testid="config-sync-pull"
          @click="onPull"
        >
          {{ t('git.toolbar.pull.label') }}
        </Button>
      </span>
    </footer>
  </div>
</template>
