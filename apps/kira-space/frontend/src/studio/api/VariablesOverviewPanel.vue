<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { PopoverContent } from '@theme/components/ui/popover';
import { tableCellClass, tableRowClass } from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { matchesQuery } from '@workbench/behaviour/match';
import { computed, ref } from 'vue';
import { useVariableRows } from './state/apiQueries';
import { useCollectionsStore } from './state/collections';
import { overviewRowsOf, useVariablesStore, type VariableOverviewRow } from './state/variables';
import { openVariableSetTab } from './tabs';

const collectionsStore = useCollectionsStore();
const variablesStore = useVariablesStore();

// A read-only popover over the already-merged data (`overviewRows`) — one panel,
// reachable from any request tab (HttpRequestView.vue and GrpcRequestView.vue both mount this
// beside EnvironmentSelect in #toolbar-2), so a user never has to open the collection's or the
// environment's own tab just to check what a `{{name}}` in front of them resolves to right now.
//
// No reveal, at all: a secret's plaintext is not in the renderer to begin with, and adding a third
// reveal surface after the row table and Copy as curl is exactly the surface-count growth the two
// rounds of findings were about. A secret row shows a `secret` chip and nothing else.
// `canEdit` defaults true (every existing caller renders byte-identically) — an
// incognito tab passes false, hiding the two Edit buttons below (the only route out of the tab
// into a persisting editor) while leaving the panel itself fully readable, as it already is by
// design.
const props = withDefaults(
  defineProps<{ collectionId: string; environmentId: string; canEdit?: boolean }>(),
  { canEdit: true },
);
const emit = defineEmits<{ close: [] }>();

const colRows = useVariableRows('collection', () => props.collectionId);
const envRows = useVariableRows('environment', () => props.environmentId);
const rows = computed<VariableOverviewRow[]>(() =>
  overviewRowsOf(colRows.data.value ?? [], envRows.data.value ?? []),
);

// P16 D14's rule, restated here (D20): name-only, never value — a value-matching filter over a
// secret-carrying list is an oracle (P16's own §5), and that reasoning applies verbatim to a panel
// that merges secrets from two scopes at once.
const filterQuery = ref('');
const isFiltered = computed(() => filterQuery.value.trim() !== '');
const filteredRows = computed(() => {
  return rows.value.filter((row) => matchesQuery(row.name, filterQuery.value));
});

function reference(name: string): string {
  return `{{${name}}}`;
}
const copyError = ref<string | null>(null);
function onCopy(name: string): void {
  copyError.value = null;
  void copyOrReportError(reference(name), (message) => {
    copyError.value = message;
  });
}

const collectionName = computed(
  () => collectionsStore.collectionRecord(props.collectionId)?.name ?? '',
);
const environmentName = computed(
  () => variablesStore.environments.find((e) => e.id === props.environmentId)?.name ?? '',
);
// P18 D17: the environment's own colour, beside "Edit environment variables…" — this panel
// already names the environment there (P17 D20).
const environmentColor = computed(
  () => variablesStore.environments.find((e) => e.id === props.environmentId)?.color ?? 'none',
);

function close(): void {
  emit('close');
}

function editCollectionVariables(): void {
  if (!props.collectionId) return;
  openVariableSetTab('collection', props.collectionId, collectionName.value);
  close();
}
function editEnvironmentVariables(): void {
  if (!props.environmentId) return;
  openVariableSetTab('environment', props.environmentId, environmentName.value);
  close();
}
</script>

<template>
  <PopoverContent align="start" class="w-96 gap-0 p-0" data-testid="variables-overview">
    <div class="flex max-h-105 flex-col">
      <SearchField
        v-model="filterQuery"
        :placeholder="t('studio.api.common.filterByName.placeholder')"
        data-testid="variables-overview-filter"
      />

      <Alert v-if="copyError" variant="error" data-testid="variables-overview-copy-error">
        <AlertDescription>{{ copyError }}</AlertDescription>
      </Alert>

      <div class="flex flex-col gap-0.5 overflow-y-auto p-1">
        <InlineState
          v-if="isFiltered && filteredRows.length === 0"
          kind="empty"
          data-testid="variables-overview-empty"
          >{{
            t('common.state.noMatches.empty')
          }}</InlineState
        >
        <InlineState
          v-else-if="rows.length === 0"
          kind="empty"
          data-testid="variables-overview-empty"
          >{{
            t('studio.api.variablesOverview.empty')
          }}</InlineState
        >
        <!-- VariableRow.vue's own grid template, minus the columns a read-only popover
             has no use for (handle, secret toggle, history, remove) -- name, value, scope,
             description, in the DOM order below. `description` is the last column specifically
             because it is the only one of the four that renders conditionally: a missing
             trailing grid item leaves its own cell empty rather than shifting `scope` into
             its place, which an *earlier* optional column would.
             grid-cols-[1.2fr_2fr_auto_1.5fr] -- an arbitrary value: a pre-existing value relocated
             into Tailwind's own syntax, not a new one. -->
        <div
          v-for="row in filteredRows"
          :key="`${row.scope}:${row.id}`"
          class="grid min-w-0 grid-cols-[1.2fr_2fr_auto_1.5fr] items-center gap-1"
          :class="[tableRowClass, tableCellClass, { 'opacity-50': row.shadowed }]"
          data-testid="variables-overview-row"
          :data-scope="row.scope"
          :data-shadowed="row.shadowed"
        >
          <Tooltip>
            <TooltipTrigger as-child>
              <Button
                variant="toolbar"
                size="inline"
                class="min-w-0 justify-start font-data font-normal text-fg hover:bg-transparent"
                data-testid="variables-overview-name"
                @click="onCopy(row.name)"
                ><span class="truncate">{{ reference(row.name) }}</span></Button
              >
            </TooltipTrigger>
            <TooltipContent>{{ t('common.action.copy.label') }}</TooltipContent>
          </Tooltip>
          <Badge v-if="row.isSecret" variant="warn" data-testid="variables-overview-secret">{{
            t('common.secret.label')
          }}</Badge>
          <span v-else class="min-w-0 text-subtle" data-testid="variables-overview-value"
            ><TruncatedText :text="row.value" /></span
          >
          <Tooltip v-if="row.shadowed">
            <TooltipTrigger as-child>
              <Badge
                class="justify-self-start"
                :variant="row.scope === 'environment' ? 'info' : 'default'"
                data-testid="variables-overview-scope"
                >{{
                  row.scope
                }}</Badge
              >
            </TooltipTrigger>
            <TooltipContent>{{ t('studio.api.variablesOverview.shadowed.hint') }}</TooltipContent>
          </Tooltip>
          <Badge
            v-else
            class="justify-self-start"
            :variant="row.scope === 'environment' ? 'info' : 'default'"
            data-testid="variables-overview-scope"
            >{{
              row.scope
            }}</Badge
          >
          <span
            v-if="row.description"
            class="min-w-0 text-subtle text-kira-sm"
            data-testid="variables-overview-description"
            ><TruncatedText :text="row.description" /></span
          >
        </div>
      </div>

      <div class="flex flex-col gap-0.5 p-1">
        <Button
          v-if="canEdit"
          variant="link"
          size="inline"
          class="justify-start gap-1 font-normal"
          :disabled="!collectionId"
          data-testid="variables-overview-edit-collection"
          @click="editCollectionVariables"
        >
          {{ t('studio.api.variablesOverview.editCollection.label') }}
        </Button>
        <Button
          v-if="canEdit"
          variant="link"
          size="inline"
          class="justify-start gap-1 font-normal"
          :disabled="!environmentId"
          data-testid="variables-overview-edit-environment"
          @click="editEnvironmentVariables"
        >
          <span
            v-if="environmentId"
            :class="colorMarkClass('dot', environmentColor)"
            data-testid="variables-overview-environment-dot"
          />
          {{ t('studio.api.variablesOverview.editEnvironment.label') }}
        </Button>
      </div>
    </div>
  </PopoverContent>
</template>
