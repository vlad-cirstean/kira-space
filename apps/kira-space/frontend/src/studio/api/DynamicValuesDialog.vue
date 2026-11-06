<script setup lang="ts">
import { FAKE_NAMES, loadDynamicGenerator } from '@kira/api-core';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { matchesQuery } from '@workbench/behaviour/match';
import { errorText } from '@workbench/bridge/codedError';
import { computed, onMounted, reactive, ref } from 'vue';
import { useDynamicValuesStore } from './state/dynamicValues';

const dynamicValuesStore = useDynamicValuesStore();

// A read-only discovery surface for the dynamic-value catalogue — nothing here edits,
// saves, or reaches Go. `catalog.ts` carries names only (no description strings, "the sample
// is the description" — a generated example says more precisely what Postman's own docs would in
// a sentence, for one call to a record this dialog is loading anyway).
const samples = reactive<Record<string, string>>({});

// One vocabulary. This dialog is the app's own reference sheet for what to type, so it now
// teaches only the spelling the app offers. The `$name` catalogue is unchanged and still resolves —
// it is no longer taught, and a Postman import rewrites what it can up front.
interface CatalogueEntry {
  name: string;
}
const ALL_ENTRIES: CatalogueEntry[] = FAKE_NAMES.map((name): CatalogueEntry => ({ name }));

// Awaits loadDynamicGenerator() on open — a user-initiated action, exactly like *Generate
// data…*'s own first open, and the same memoised promise a send would use. One sample per name,
// freshly generated every time the dialog opens (closing and reopening shows a different one).
// `generate` already accepts either spelling (generators.ts's own dispatch).
const error = ref<string | null>(null);
onMounted(async () => {
  try {
    const generate = await loadDynamicGenerator();
    for (const entry of ALL_ENTRIES) {
      samples[entry.name] = generate(entry.name) ?? '';
    }
  } catch (err) {
    error.value = t('studio.api.dynamicValues.loadError.error', { error: errorText(err) });
  }
});

function reference(name: string): string {
  return `{{${name}}}`;
}

// P16 D15: matches the name or its generated sample — a filter over the flat, merged list.
const filterQuery = ref('');
const isFiltered = computed(() => filterQuery.value.trim() !== '');
const filteredEntries = computed(() => {
  const q = filterQuery.value;
  return ALL_ENTRIES.filter(
    (entry) => matchesQuery(entry.name, q) || matchesQuery(samples[entry.name] ?? '', q),
  );
});

function onCopy(name: string): void {
  error.value = null;
  void copyOrReportError(reference(name), (message) => {
    error.value = message;
  });
}

function close(): void {
  dynamicValuesStore.closeDynamicValuesDialog();
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && close()">
    <DialogContent size="md" data-testid="dynamic-values-dialog">
      <DialogHeader close-testid="dynamic-values-dialog-close">
        <DialogTitle>{{ t('studio.api.dynamicValues.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody class="gap-1.5">
        <SearchField
          v-model="filterQuery"
          size="kira-lg"
          :placeholder="t('common.action.filter.label')"
          data-testid="dynamic-values-filter"
        />
        <Alert v-if="error" variant="error" data-testid="dynamic-values-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <EmptyState
          v-if="isFiltered && filteredEntries.length === 0"
          data-testid="dynamic-values-filter-empty"
          :icon="ICON.search"
          :title="t('common.state.noMatches.empty')"
        />
        <Tooltip v-for="entry in filteredEntries" :key="entry.name">
          <TooltipTrigger as-child>
            <Button
              variant="toolbar"
              size="kira"
              class="h-auto min-h-6.5 w-full justify-between gap-1 px-1.5 py-1 text-left text-kira-md font-normal text-fg"
              data-testid="dynamic-values-fake-row"
              :data-name="entry.name"
              @click="onCopy(entry.name)"
            >
              <code class="shrink-0 text-fg font-data" data-testid="dynamic-values-reference">{{
                reference(entry.name)
              }}</code>
              <Badge variant="info" class="min-w-0 truncate" data-testid="dynamic-values-sample">{{
                samples[entry.name] ?? ''
              }}</Badge>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('common.action.copy.label') }}</TooltipContent>
        </Tooltip>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="dynamic-values-close" @click="close">{{
          t('common.action.close.label')
        }}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
