<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useCollectionsStore } from './state/collections';

const collectionsStore = useCollectionsStore();

// The import report is part of the feature, not decoration. Every warning kind is a case
// where the app quietly does something other than what the file said — a script that is kept but
// never run, an auth block whose values are dropped and never applied, a GraphQL body imported as
// JSON, a file referenced by a name from another machine — and the alternative to saying so here
// is letting the user find out from a 401 or an E_BAD_REQUEST minutes later.
//
// A pre-import preview dialog was considered and declined: it is a second UI for an operation that
// is almost never wrong, and the report after the fact carries the same information at a tenth of
// the cost. Import is not undoable; deleting the collection is the undo, one context-menu
// item away.
//
// Alert with the `warn`/`note` tone variants, dynamic per warning count — the same tone
// vocabulary RawExchangePane.vue's own dynamic-tone strip uses.
const result = computed(() => collectionsStore.importResult);

const tone = computed(() => {
  const files = result.value?.files ?? [];
  if (files.some((f) => f.kind === '')) return 'error';
  const warned = files.some((f) => (f.collection ?? f.environment)?.warnings.length);
  return warned ? 'warn' : 'info';
});

interface FileLine {
  key: string;
  kind: string;
  text: string;
  warnings: { kind: string; detail: string }[];
}

const lines = computed<FileLine[]>(() =>
  (result.value?.files ?? []).map((f) => {
    if (f.collection) {
      const r = f.collection;
      return {
        key: f.path,
        kind: 'collection',
        text: t('studio.api.importReport.summary.description', {
          name: r.name,
          requests: t('studio.api.importReport.requests.label', { count: r.requests }),
          folders: t('studio.api.importReport.folders.label', { count: r.folders }),
        }),
        warnings: r.warnings,
      };
    }
    if (f.environment) {
      const r = f.environment;
      return {
        key: f.path,
        kind: 'environment',
        text: t('studio.api.importReport.environment.description', {
          name: r.name,
          variables: t('studio.api.importReport.variables.label', { count: r.variables }),
        }),
        warnings: r.warnings,
      };
    }
    return {
      key: f.path,
      kind: 'failed',
      text: t('studio.api.importReport.failed.error', { file: f.fileName, detail: f.error ?? '' }),
      warnings: [],
    };
  }),
);
</script>

<template>
  <Alert v-if="result" :variant="tone" strip data-testid="import-report">
    <div class="flex min-w-0 flex-col gap-1">
      <div
        v-for="line in lines"
        :key="line.key"
        class="flex min-w-0 flex-col gap-0.5"
        data-testid="import-report-file"
        :data-kind="line.kind"
      >
        <div data-testid="import-report-summary">{{ line.text }}</div>
        <ul v-if="line.warnings.length > 0" class="m-0 flex flex-col gap-0.5 pl-2">
          <li v-for="warning in line.warnings" :key="warning.kind" :data-kind="warning.kind">
            {{ warning.detail }}
          </li>
        </ul>
      </div>
    </div>
    <AlertAction>
      <TooltipIconButton
        icon="close"
        :label="t('common.action.dismiss.label')"
        data-testid="import-report-dismiss"
        @click="collectionsStore.dismissReport"
      />
    </AlertAction>
  </Alert>
  <!-- P5 D16: the export path's own strip, independent of the import one above (a session can
       export without ever having imported). -->
  <Alert v-if="collectionsStore.exportWarning" variant="warn" strip data-testid="export-warning">
    <div class="flex min-w-0 flex-col gap-0.5">{{ collectionsStore.exportWarning }}</div>
    <AlertAction>
      <TooltipIconButton
        icon="close"
        :label="t('common.action.dismiss.label')"
        data-testid="export-warning-dismiss"
        @click="collectionsStore.dismissExportWarning"
      />
    </AlertAction>
  </Alert>
  <!-- P108 F10: every tree mutation (create/rename/delete/duplicate/save/import/export) used to
       let its own failure throw uncaught from a fire-and-forget `void` call — nothing told the
       user why a row didn't change. -->
  <Alert v-if="collectionsStore.error" variant="error" strip data-testid="collections-error">
    <div class="flex min-w-0 flex-col gap-0.5">{{ collectionsStore.error }}</div>
    <AlertAction>
      <TooltipIconButton
        icon="close"
        :label="t('common.action.dismiss.label')"
        data-testid="collections-error-dismiss"
        @click="collectionsStore.dismissError"
      />
    </AlertAction>
  </Alert>
  <!-- P112 §4.3: a failed tree List used to reject into a void'd promise with nothing shown — this
       is the read side's own strip, alongside the mutation-failure one above. No dismiss: it
       clears itself once the query's own retry/next refetch succeeds. -->
  <Alert
    v-if="collectionsStore.treeLoadError"
    variant="error"
    strip
    data-testid="collections-tree-load-error"
  >
    <AlertDescription>{{ collectionsStore.treeLoadError }}</AlertDescription>
  </Alert>
</template>
