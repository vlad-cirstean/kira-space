<script setup lang="ts">
import { definitionText } from '@shared/domain/definition';
import { decodePath, pathTail } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RunState from '@theme/RunState.vue';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { matchesQuery } from '@workbench/behaviour/match';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { findRanges } from '../../editor/findRanges';
import MonacoHost from '../../editor/MonacoHost.vue';
import { useConnectionsStore } from '../../state/connections';
import { nodeKindLabel } from '../../state/nodeKindLabel';
import { useRunState } from '../../state/runState';
import type { DefinitionTabRecord } from '../../state/tabDomain';
import { useTabsStore } from '../../state/tabs';
import EngineIcon from '../../theme/EngineIcon.vue';
import ResponseFindBar, {
  type FindBarHost,
  type FindBarTarget,
} from '../shared/ResponseFindBar.vue';
import StaleDataStrip from '../shared/StaleDataStrip.vue';
import { sqlDialectFor } from '../shared/sqlIdent';
import { refreshOrReconnect, useConnectionGate } from '../shared/useConnectionGate';
import ColumnsSection from './ColumnsSection.vue';
import ConstraintsSection from './ConstraintsSection.vue';
import IndexesSection from './IndexesSection.vue';
import PropertiesSection from './PropertiesSection.vue';
import { useDefinitionViewStore } from './state';
import { buildConstraintRows } from './structure';
import ValidationSection from './ValidationSection.vue';

// MainView.vue keys this component by tab.id, so one instance <-> one tab — same discipline as
// DataView.vue.
const props = defineProps<{ tab: DefinitionTabRecord }>();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const definitionViewStore = useDefinitionViewStore();

const { needsReconnect, reconnecting, onReconnectAndLoad } = useConnectionGate(
  () => props.tab,
  () => definitionViewStore.load(props.tab.id),
);

const rt = computed(() => definitionViewStore.runtime[props.tab.id]);
const loading = computed(() => rt.value?.status === 'loading');
const stale = computed(() => definitionViewStore.isStale(props.tab.id));

function onRefresh(): void {
  refreshOrReconnect(needsReconnect.value, onReconnectAndLoad, () =>
    definitionViewStore.load(props.tab.id, { refresh: true }),
  );
}

// "Open query console" mirrors project/menus.ts's own consoleMenuItem: the tab's own connection
// and path, handed straight to openConsoleTab, no new capability involved.
function onOpenConsole(): void {
  if (props.tab.connectionId) tabsStore.openConsoleTab(props.tab.connectionId, props.tab.path);
}

const copyError = ref<string | null>(null);
function onCopy(): void {
  if (!definition.value) return;
  void copyOrReportError(
    definitionText(definition.value),
    (m) => {
      copyError.value = m;
    },
    () => {
      copyError.value = null;
    },
  );
}

let unregisterCommands: Array<() => void> = [];

onMounted(() => {
  if (!needsReconnect.value && !definitionViewStore.runtime[props.tab.id]) {
    void definitionViewStore.load(props.tab.id);
  }
  unregisterCommands = [
    registerCommand('view.refresh', onRefresh),
    registerCommand('view.find', toggleSearch),
  ];
});

onUnmounted(() => {
  for (const off of unregisterCommands) off();
});

const targetTail = computed(() => pathTail(props.tab.path));
const targetLabel = computed(() => targetTail.value?.name ?? props.tab.path);

const definition = computed(() => rt.value?.definition ?? null);
const meta = computed(() => rt.value?.meta ?? null);
const document = computed(() => (definition.value ? definitionText(definition.value) : ''));

// D7: Structure (default) vs Source, persisted per tab; `.default('structure')` (tabs.ts) keeps
// a tab saved under the pre-P19 empty `{}` shape restorable.
const pane = computed(() => props.tab.state.pane);
const PANE_OPTIONS = [
  {
    value: 'structure' as const,
    label: t('studio.definition.pane.structure.option'),
    testid: 'definition-pane-structure',
  },
  {
    value: 'source' as const,
    label: t('common.field.source.label'),
    testid: 'definition-pane-source',
  },
];
function setPane(value: 'structure' | 'source'): void {
  tabsStore.patchDefinitionTabState(props.tab.id, { pane: value });
}

// Mongo collections have documents, not columns/constraints (D9/D11's scope is SQL relations);
// Indexes is the one section every kind shares (P8's D10).
const isCollection = computed(() => definition.value?.kind === 'collection');
const foreignKeyColumnNames = computed(
  () => new Set(meta.value?.foreignKeys.flatMap((fk) => fk.columns) ?? []),
);
const constraintRows = computed(() =>
  definition.value && meta.value ? buildConstraintRows(definition.value, meta.value) : [],
);

// A whole DDL document plus columns/indexes/constraints sections is the single largest searchable
// document in Studio, and had no search at all. Structure's own search is a plain substring filter
// over each section's *rows*, applied here (once, over the arrays every section already receives
// as props) rather than teaching four section components their own filtering —
// ColumnsSection/IndexesSection/ConstraintsSection stay unchanged.
const searchOpen = computed({
  get: () => rt.value?.searchOpen ?? false,
  set: (v: boolean) => definitionViewStore.setSearchOpen(props.tab.id, v),
});
const structureFilterQuery = computed({
  get: () => rt.value?.structureFilter ?? '',
  set: (v: string) => definitionViewStore.setStructureFilter(props.tab.id, v),
});
function toggleSearch(): void {
  searchOpen.value = !searchOpen.value;
  // D13's own rule (HttpRequestView.vue's toggleFieldFilter): closing must restore every hidden
  // row — the find bar's own query lives inside ResponseFindBar and resets for free on unmount.
  if (!searchOpen.value) structureFilterQuery.value = '';
}
function closeSearch(): void {
  searchOpen.value = false;
  structureFilterQuery.value = '';
}

const structureFilterActive = computed(
  () => searchOpen.value && pane.value === 'structure' && structureFilterQuery.value.trim() !== '',
);
const filteredColumns = computed(() => {
  if (!structureFilterActive.value) return meta.value?.columns ?? [];
  const q = structureFilterQuery.value;
  return (meta.value?.columns ?? []).filter((c) => matchesQuery(c.name, q));
});
const filteredIndexes = computed(() => {
  if (!structureFilterActive.value) return meta.value?.indexes ?? [];
  const q = structureFilterQuery.value;
  return (meta.value?.indexes ?? []).filter(
    (i) => matchesQuery(i.name, q) || (i.columns ?? []).some((c) => matchesQuery(c, q)),
  );
});
const filteredConstraintRows = computed(() => {
  if (!structureFilterActive.value) return constraintRows.value;
  const q = structureFilterQuery.value;
  return constraintRows.value.filter((r) => matchesQuery(r.name, q));
});

// Source's own search is the same find-in-document ResponseFindBar every other big document in
// this app uses (HTTP's ResponsePane.vue, gRPC's own ResponsePane.vue) — one target, the whole
// DDL/JSON text.
const docHostRef = ref<FindBarHost | null>(null);
const findBarRef = ref<{ query: string; currentGlobal: number } | null>(null);
const findTargets = computed<readonly FindBarTarget[]>(() => {
  if (!searchOpen.value || pane.value !== 'source') return [];
  return [{ doc: document.value, host: docHostRef.value }];
});
const docHighlights = computed(() => {
  const bar = findBarRef.value;
  const query = bar?.query ?? '';
  if (!query || findTargets.value.length === 0) return undefined;
  const currentGlobal = bar?.currentGlobal ?? -1;
  return (doc: string) => findRanges(doc, query, currentGlobal);
});

const dialect = computed(() =>
  sqlDialectFor(connectionsStore.connectionRecord(props.tab.connectionId)?.kind),
);

const readOnlyLabel = computed(() =>
  definition.value?.origin === 'server'
    ? t('studio.definition.readOnlyServer.status')
    : t('studio.definition.readOnlyComposed.status'),
);

// Open in console was unconditional, which only ever mattered because every kind with a definition
// tab also had a console (Postgres/MariaDB/Mongo). Kafka and SQS break that coincidence (caps.sql
// is false for both) — same gate project/menus.ts's own consoleMenuItem() already uses for the
// tree's context-menu equivalent.
const canOpenConsole = computed(
  () => connectionsStore.states[props.tab.connectionId ?? '']?.caps?.sql === true,
);

// P16 design system LAW: connection colour reaches a view as a rail (tree, tab, toolbar
// cap) or a dot (view header) — the same per-tab lookup Toolbar.vue and TreeRow.vue already
// use for the rail elsewhere, just aimed at the dot instead.
const connRecord = computed(() => connectionsStore.connectionRecord(props.tab.connectionId));
// P104 §3: ViewChrome/ViewHeader/RunState inlined at this call site (no library counterpart) —
// railColor mirrors ViewChrome.vue's own `connection ? (connection.color ?? null) : undefined`.
const railColor = computed(() => (connRecord.value ? (connRecord.value.color ?? null) : undefined));
const runState = useRunState(() => props.tab.id);
// Produced locally from the path — the same discipline SlickGridHost.vue's own qualifiedName()
// uses (never round-tripped to the engine for a string join): connection name plus every
// segment above the target, joined for the view header's breadcrumb.
const breadcrumb = computed(() => {
  if (!props.tab.connectionId) return '';
  const parents = decodePath(props.tab.connectionId, props.tab.path)
    .segments.slice(0, -1)
    .map((s) => s.name);
  const parts = [connRecord.value?.name, ...parents].filter((p): p is string => !!p);
  return parts.length > 0 ? `${parts.join(' / ')} / ` : '';
});
</script>

<template>
  <div
    class="h-full flex flex-col min-h-0"
    data-testid="definition-view"
    :data-path="tab.path"
    :data-origin="definition?.origin ?? ''"
    :data-source="rt?.source ?? ''"
    data-read-only-reason="definition-not-editable"
  >
    <!-- P104 §3: ViewChrome/ViewHeader inlined (no library counterpart) — Tailwind utilities over
         components/ui parts, the same view-head/toolbar chrome utility set every other view uses
         (P110 B28). -->
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span v-if="connRecord?.kind" class="size-4 flex items-center justify-center shrink-0"
        ><EngineIcon :kind="connRecord.kind" :size="13" /></span
      >
      <span class="size-4 flex items-center justify-center shrink-0"
        ><CodiconIcon name="code" :size="13" /></span
      >
      <span class="text-kira-md text-fg truncate" data-testid="definition-target">
        <span v-if="breadcrumb" class="text-subtle">{{ breadcrumb }}</span>{{ targetLabel }}
      </span>
      <Badge v-if="targetTail">{{ nodeKindLabel(targetTail.kind) }}</Badge>
      <Badge>
        <CodiconIcon name="lock" :size="13" />
        {{ readOnlyLabel }}
      </Badge>
      <span class="ml-auto flex items-center gap-1" />
    </ViewToolbar>
    <div :class="colorMarkClass('band', railColor)" />
    <ViewToolbar>
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          :icon="ICON.refresh"
          :label="t('common.action.refresh.label')"
          disabled-trigger
          :disabled="loading"
          data-testid="definition-refresh"
          @click="onRefresh"
        />
      </div>
      <ToolbarSeparator />
      <div class="flex items-center gap-1.5 min-w-0">
        <SecondaryTabs
          :model-value="pane"
          :items="PANE_OPTIONS"
          data-testid="definition-pane"
          @update:model-value="(v) => setPane(v as 'structure' | 'source')"
        />
      </div>
      <!-- P22b D14: the single largest searchable document in Studio (F21) had no search at
           all — a find-in-document bar for Source, a plain substring filter for Structure. -->
      <TooltipIconButton
        :icon="ICON.search"
        :label="
          pane === 'source'
            ? t('studio.definition.search.source.tooltip')
            : t('studio.definition.structureFilter.placeholder')
        "
        :aria-label="t('common.action.search.label')"
        :data-active="searchOpen"
        data-testid="definition-search-toggle"
        @click="toggleSearch"
      />
      <span class="ml-auto" />
      <RunState :state="runState" />
      <div class="flex items-center gap-1.5 min-w-0">
        <!-- D7: Copy/notes describe the Source pane's raw text specifically — Structure has its
             own per-section content and count badges, nothing to copy as one document. -->
        <template v-if="pane === 'source'">
          <ToolbarSeparator />
          <Tooltip>
            <TooltipTrigger as-child>
              <Button variant="toolbar" size="kira" data-testid="definition-copy" @click="onCopy">
                <CodiconIcon :name="ICON.copy" :size="13" />
                {{ t('common.action.copy.label') }}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{{ t('studio.definition.copy.tooltip') }}</TooltipContent>
          </Tooltip>
        </template>
        <Tooltip v-if="canOpenConsole">
          <TooltipTrigger as-child>
            <Button
              variant="toolbar"
              size="kira"
              data-testid="definition-open-console"
              @click="onOpenConsole"
            >
              <CodiconIcon name="terminal" :size="13" />
              {{ t('studio.definition.openConsole.label') }}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('studio.definition.openConsole.tooltip') }}</TooltipContent>
        </Tooltip>
      </div>
    </ViewToolbar>

    <Alert
      strip
      v-if="rt?.status === 'error' && rt.error"
      variant="error"
      data-testid="definition-error"
    >
      <AlertDescription
        ><span class="whitespace-pre-wrap font-data">{{ rt.error }}</span></AlertDescription
      >
    </Alert>
    <Alert strip v-if="copyError" variant="error" data-testid="definition-copy-error">
      <AlertDescription>{{ copyError }}</AlertDescription>
    </Alert>
    <Alert
      v-if="pane === 'source' && definition && definition.notes.length > 0"
      strip
      variant="info"
      data-testid="definition-notes"
    >
      <AlertDescription>
        <ul class="m-0 pl-3">
          <li v-for="(note, i) in definition.notes" :key="i">{{ note }}</li>
        </ul>
      </AlertDescription>
    </Alert>
    <SearchField
      v-if="searchOpen && pane === 'structure'"
      v-model="structureFilterQuery"
      :placeholder="t('studio.definition.structureFilter.placeholder')"
      data-testid="definition-structure-filter"
    />

    <!-- The reconnect gate used to replace this whole view chrome (header, toolbar and
         all) — every other view but the grid's DataView.vue did the same, the one inconsistency
         this fixes. The chrome above always renders; only the body — the part that needs
         a live connection — swaps for the gate. -->
    <StaleDataStrip
      v-if="stale && rt?.definition"
      testid="definition-stale"
      :reconnecting="reconnecting"
      @reconnect="onReconnectAndLoad"
    />
    <EmptyState
      v-if="needsReconnect && !rt?.definition"
      data-testid="definition-reconnect"
      icon="plug"
      :title="t('studio.common.disconnected.empty')"
    >
      <Button
        variant="dialog-primary"
        size="kira-lg"
        data-testid="definition-reconnect-load"
        :pending="reconnecting"
        @click="onReconnectAndLoad"
      >
        {{ t('studio.common.reconnectAndLoad.label') }}
      </Button>
    </EmptyState>
    <template v-else>
      <div v-if="pane === 'source'" class="flex-1 min-h-0">
        <MonacoHost
          ref="docHostRef"
          :doc="document"
          :language="definition?.language === 'json' ? 'json' : 'sql'"
          :sql-dialect="dialect"
          readonly
          :range-highlights="docHighlights"
        />
      </div>
      <!-- The Structure body no longer hard-requires `meta` — Kafka and SQS have no describe(), so
         a definition can arrive with meta still null. PropertiesSection renders regardless;
         everything below it stays conditional on the data it needs, exactly as before.
         Columns/Indexes/Constraints get the filtered arrays (computed above) instead of meta's own
         raw ones — filtering lives here, once, not in each section. -->
      <div v-else-if="definition" class="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 p-3">
        <PropertiesSection
          v-for="section in definition.sections"
          :key="section.title"
          :section="section"
        />
        <template v-if="meta">
          <ColumnsSection
            v-if="!isCollection"
            :columns="filteredColumns"
            :foreign-key-column-names="foreignKeyColumnNames"
            :connection-id="tab.connectionId ?? ''"
            :table-path="tab.path"
            @copy-error="(m: string) => (copyError = m)"
          />
          <IndexesSection :indexes="filteredIndexes" />
          <ConstraintsSection
            v-if="!isCollection"
            :connection-id="tab.connectionId ?? ''"
            :constraints="filteredConstraintRows"
          />
        </template>
        <ValidationSection v-if="isCollection" :document-schema="definition.documentSchema" />
      </div>
      <!-- P22b D14: docked below the pane it searches (LAW 03), mirroring HTTP's own
         ResponsePane.vue — only shown over the Source pane's single document. -->
      <ResponseFindBar
        v-if="searchOpen && pane === 'source'"
        ref="findBarRef"
        :targets="findTargets"
        @close="closeSearch"
      />
      <!-- LAW — there is no editor status line: identity moved to the view header above,
         duration to the toolbar's run-state, and this tab has no pending edits to report. -->
    </template>
  </div>
</template>
