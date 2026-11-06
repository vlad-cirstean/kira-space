<script setup lang="ts">
import { pathTail } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import RunState from '@theme/RunState.vue';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue';
import type { ConsoleTabRecord } from '../../../state/tabDomain';
import MonacoHost from '../../editor/MonacoHost.vue';
import { useRunState } from '../../state/runState';
import EngineIcon from '../../theme/EngineIcon.vue';
import CellEditorDock from '../shared/celleditor/CellEditorDock.vue';
import SearchToolbar from '../shared/page/SearchToolbar.vue';
import StaleDataStrip from '../shared/StaleDataStrip.vue';
import { useConnectionGate } from '../shared/useConnectionGate';
import ConsoleResultGrid from './ConsoleResultGrid.vue';
import ConsoleResultStrip from './ConsoleResultStrip.vue';
import ConsoleSavedMenu from './ConsoleSavedMenu.vue';
import ExplainResultView from './ExplainResultView.vue';
import { canFormatConsole } from './format';
import { getPage } from './resultPages';
import { type Match, pageSearchApi } from './search';
import { setNewResultSet, setText, useConsoleViewStore } from './state';
import { useConsoleFormat } from './useConsoleFormat';
import { useConsoleLanguage } from './useConsoleLanguage';
import { useConsoleRun } from './useConsoleRun';
import { useConsoleUiStore } from './viewState';

const consoleViewStore = useConsoleViewStore();

// MainView.vue keys this component by tab.id — same discipline as DefinitionView.vue/DataView.vue.
const props = defineProps<{ tab: ConsoleTabRecord }>();
const tabRef = computed(() => props.tab);

// A console tab hydrates without loading anything, so no onLoad is passed.
const { needsReconnect, reconnecting, onReconnectAndLoad } = useConnectionGate(() => props.tab);

// Results are read-only output: reconnecting only restores the session, never reruns a statement.
async function onStaleReconnect(): Promise<void> {
  if (await onReconnectAndLoad()) consoleViewStore.restamp(props.tab.id);
}

const rt = computed(() => consoleViewStore.runtime[props.tab.id]);
const running = computed(() => rt.value?.status === 'running');
const hasResults = computed(() => (rt.value?.results.length ?? 0) > 0);
// Result pages outlived their connection: kept, behind StaleDataStrip.
const stale = computed(() => consoleViewStore.isStale(props.tab.id));

const targetTail = computed(() => pathTail(props.tab.path));
const runState = useRunState(() => props.tab.id);

const {
  railColor,
  connectionKind,
  dialect,
  splitOptionsFor,
  language,
  completionSources,
  lintSource,
  hoverSource,
} = useConsoleLanguage(tabRef);

// Caret and scroll outlive the component: a tab switch unmounts it.
const uiState = useConsoleUiStore().ui(props.tab.id);
const restored = { cursor: uiState.cursor, scrollTop: uiState.scrollTop };
const cursorPos = ref(restored.cursor);
const saveScrollTop = useDebounceFn((top: number) => {
  uiState.scrollTop = top;
}, TIMING.scrollPersist);
const savedMenuOpen = ref(false);
const savedMenuTriggerEl = ref<{ $el: HTMLElement } | null>(null);
const canFormat = computed(() => canFormatConsole(connectionKind.value));

// Typed as the bare exposed shape so the ref does not read as a type-only use of the MonacoHost import.
const editorHost = ref<{
  focus: () => void;
  setCursor: (pos: number) => void;
  setDoc: (text: string) => void;
} | null>(null);

const {
  starting,
  connectError,
  runBlocked,
  canExplain,
  explainTooltip,
  explainError,
  runStatement,
  runAll,
  onExplain,
} = useConsoleRun({
  tab: tabRef,
  running,
  needsReconnect,
  onReconnectAndLoad,
  connectionKind,
  dialect,
  splitOptionsFor,
  cursorPos,
});

const { formatError, formatWarning, formatNote, onFormat, clearFormatMessages } = useConsoleFormat({
  tab: tabRef,
  connectionKind,
  canFormat,
  dialect,
  splitOptionsFor,
  cursorPos,
  editorHost,
  resetStalePreviewState: () => resetStalePreviewState(),
});

const autoExplainMessage = computed(() => {
  const state = rt.value?.autoExplain;
  if (!state) return '';
  if (state.kind === 'truncated') {
    return t('studio.console.autoExplain.truncated.body');
  }
  const worst = state.plans[state.worstIndex]?.plan;
  if (!worst) return '';
  const parts: string[] = [];
  if (worst.estimatedRowsRead !== undefined) {
    parts.push(t('studio.console.plan.estimatedRows.status', { count: worst.estimatedRowsRead }));
  }
  const firstWarning = worst.issues.find((i) => i.severity === 'warn');
  if (firstWarning) parts.push(firstWarning.message);
  return parts.join(' · ') || t('studio.console.autoExplain.expensive.status');
});
const canShowAutoExplainPlan = computed(() => rt.value?.autoExplain?.kind === 'plans');

function onShowAutoExplainPlan(): void {
  consoleViewStore.showAutoExplainPlan(props.tab.id);
}

// The popover unmounts its focused entry on close; without this focus falls to <body>.
function onSavedMenuClose(): void {
  savedMenuOpen.value = false;
  void nextTick().then(() => editorHost.value?.focus());
}

const localDoc = shallowRef(props.tab.state.text);
let lastEmitted = props.tab.state.text;

function resetStalePreviewState(): void {
  clearFormatMessages();
  explainError.value = null;
  connectError.value = null;
  consoleViewStore.clearAutoExplain(props.tab.id);
}

function onDocChange(text: string): void {
  lastEmitted = text;
  setText(props.tab.id, text);
  resetStalePreviewState();
}

watch(
  () => props.tab.state.text,
  (text) => {
    if (text === lastEmitted) return;
    // The editor is about to hold exactly this, so the echo guard must say so.
    lastEmitted = text;
    localDoc.value = text;
    editorHost.value?.setDoc(text);
    resetStalePreviewState();
  },
);

function onStop(): void {
  consoleViewStore.stop(props.tab.id);
}

function onToggleSearch(): void {
  consoleViewStore.toggleSearchOpen(props.tab.id);
}
function onCloseSearch(): void {
  consoleViewStore.setSearchOpen(props.tab.id, false);
}

const resultGridRef = ref<{
  goToMatch: (match: Match) => void;
  expandAll: () => void;
  collapseAll: () => void;
} | null>(null);
const activeResultTruncated = computed(() => {
  const page = getPage(rt.value?.activeKey ?? '');
  return page ? { rows: page.rowCount, truncated: page.position.truncated === true } : null;
});
const activeResultIsPlan = computed(
  () => rt.value?.results.find((r) => r.key === rt.value?.activeKey)?.kind === 'plan',
);
function onExpandAllResults(): void {
  resultGridRef.value?.expandAll();
}
function onCollapseAllResults(): void {
  resultGridRef.value?.collapseAll();
}
function onGoToMatch(match: Match): void {
  resultGridRef.value?.goToMatch(match);
}

let unregisterCommands: Array<() => void> = [];

onMounted(() => {
  unregisterCommands = [
    registerCommand('view.run', runStatement),
    registerCommand('view.run-all', runAll),
    registerCommand('view.format', onFormat),
    registerCommand('view.explain', onExplain),
    registerCommand('view.find', onToggleSearch),
  ];
});

onUnmounted(() => {
  for (const off of unregisterCommands) off();
});
</script>

<template>
  <div class="h-full flex flex-col min-h-0" data-testid="console-view" :data-path="tab.path">
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span v-if="connectionKind" class="size-4 flex items-center justify-center shrink-0">
        <EngineIcon :kind="connectionKind" :size="13" />
      </span>
      <span class="size-4 flex items-center justify-center shrink-0">
        <CodiconIcon name="terminal" :size="13" />
      </span>
      <span class="text-kira-md text-fg truncate" data-testid="console-target">{{
        targetTail?.name ?? tab.path ?? t('studio.console.target.fallback.label')
      }}</span>
    </ViewToolbar>

    <div :class="colorMarkClass('band', railColor)" />
    <ViewToolbar>
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          icon="refresh"
          :label="t('common.action.refresh.label')"
          disabled-trigger
          data-testid="console-refresh"
          :disabled="!needsReconnect"
          @click="onReconnectAndLoad"
        />
        <TooltipIconButton
          icon="debug-stop"
          :label="t('common.action.stop.label')"
          disabled-trigger
          :class="{ 'text-error': running }"
          data-testid="console-stop"
          :disabled="!running"
          @click="onStop"
        />
      </div>
      <Tooltip>
        <TooltipTrigger as-child>
          <TooltipDisabledTrigger>
            <Button
              variant="dialog-primary"
              size="kira"
              data-testid="console-run-statement"
              :pending="running || starting"
              @click="runStatement"
            >
              <CodiconIcon name="play" :size="13" />
              {{ t('common.action.run.label') }}
            </Button>
          </TooltipDisabledTrigger>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.console.run.tooltip') }}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger as-child>
          <TooltipDisabledTrigger>
            <Button
              variant="toolbar"
              size="kira"
              data-testid="console-run-all"
              :disabled="runBlocked"
              @click="runAll"
            >
              <CodiconIcon name="run-all" :size="13" />
              {{ t('common.action.runAll.label') }}
            </Button>
          </TooltipDisabledTrigger>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.console.runAll.tooltip') }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="canFormat">
        <TooltipTrigger as-child>
          <TooltipDisabledTrigger>
            <Button
              variant="toolbar"
              size="kira"
              data-testid="console-format"
              :disabled="!tab.state.text.trim()"
              @click="onFormat"
            >
              <CodiconIcon name="indent" :size="13" />
              {{ t('studio.console.format.label') }}
            </Button>
          </TooltipDisabledTrigger>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.console.format.tooltip') }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="dialect">
        <TooltipTrigger as-child>
          <TooltipDisabledTrigger>
            <Button
              variant="toolbar"
              size="kira"
              data-testid="console-explain"
              :disabled="!canExplain || runBlocked"
              @click="onExplain"
            >
              <CodiconIcon name="list-tree" :size="13" />
              {{ t('studio.console.explain.label') }}
            </Button>
          </TooltipDisabledTrigger>
        </TooltipTrigger>
        <TooltipContent>{{ explainTooltip }}</TooltipContent>
      </Tooltip>
      <ToolbarSeparator />
      <TooltipIconButton
        icon="layers"
        :label="
          tab.state.newResultSet
            ? t('studio.console.newResultSet.add.tooltip')
            : t('studio.console.newResultSet.replace.tooltip')
        "
        :aria-label="t('studio.console.newResultSet.toggle.aria')"
        :pressed="!tab.state.newResultSet"
        data-testid="console-new-result-toggle"
        @click="setNewResultSet(tab.id, !tab.state.newResultSet)"
      />
      <ToolbarSeparator />
      <div class="relative">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              ref="savedMenuTriggerEl"
              variant="toolbar"
              size="kira"
              data-testid="console-saved-toggle"
              @click="savedMenuOpen = !savedMenuOpen"
            >
              <CodiconIcon name="bookmark" :size="13" />
              {{ t('studio.console.savedMenu.title') }}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('studio.console.savedMenu.title') }}</TooltipContent>
        </Tooltip>
        <Popover :open="savedMenuOpen" @update:open="(v) => (savedMenuOpen = v)">
          <PopoverAnchor
            :reference="(savedMenuTriggerEl?.$el as HTMLElement) ?? undefined"
            class="hidden"
          />
          <ConsoleSavedMenu v-if="savedMenuOpen" :tab-id="tab.id" @close="onSavedMenuClose" />
        </Popover>
      </div>
      <ToolbarSeparator />
      <TooltipIconButton
        v-if="!activeResultIsPlan"
        icon="search"
        :label="t('studio.console.findInResult.label')"
        :pressed="!!rt?.searchOpen"
        data-testid="console-search"
        @click="onToggleSearch"
      />
      <span class="ml-auto" />
      <Tooltip :disabled="true">
        <TooltipTrigger as-child>
          <RunState :state="runState" />
        </TooltipTrigger>
      </Tooltip>
    </ViewToolbar>

    <Alert
      strip
      v-if="rt?.status === 'error' && rt.error"
      variant="error"
      data-testid="console-error"
    >
      <AlertDescription>{{ rt.error.message }}</AlertDescription>
    </Alert>
    <Alert strip v-if="formatError" variant="error" data-testid="console-format-error">
      <AlertDescription>{{ formatError }}</AlertDescription>
    </Alert>
    <Alert strip v-if="formatWarning" variant="warn" data-testid="console-format-warning">
      <AlertDescription>{{ formatWarning }}</AlertDescription>
    </Alert>
    <Alert strip v-if="formatNote" variant="info" data-testid="console-format-note">
      <AlertDescription>{{ formatNote }}</AlertDescription>
    </Alert>
    <Alert strip v-if="connectError" variant="error" data-testid="console-connect-error">
      <AlertDescription>{{ connectError }}</AlertDescription>
    </Alert>
    <Alert strip v-if="explainError" variant="error" data-testid="console-explain-error">
      <AlertDescription>{{ explainError }}</AlertDescription>
    </Alert>
    <Alert strip v-if="rt?.autoExplain" variant="warn" data-testid="console-auto-explain">
      <AlertDescription>{{ autoExplainMessage }}</AlertDescription>
      <AlertAction v-if="canShowAutoExplainPlan">
        <Button
          variant="link"
          size="inline"
          data-testid="console-auto-explain-show-plan"
          @click="onShowAutoExplainPlan"
        >
          {{ t('studio.console.autoExplain.showPlan.label') }}
        </Button>
      </AlertAction>
    </Alert>

    <div class="flex flex-1 min-h-0 flex-col">
      <div class="flex min-h-0 flex-1 flex-col">
        <div class="flex-1 basis-2/5 min-h-0">
          <MonacoHost
            ref="editorHost"
            :doc="localDoc"
            :language="language"
            :sql-dialect="dialect"
            :autocomplete="language !== 'plain'"
            :completion-sources="completionSources"
            :lint-source="lintSource"
            :hover-source="hoverSource"
            keep-selection-on-external-sync
            @update:doc="onDocChange"
            :cursor="restored.cursor"
            :scroll-top="restored.scrollTop"
            @update:cursor="
              cursorPos = $event;
              uiState.cursor = $event;
            "
            @update:scroll-top="saveScrollTop"
          />
        </div>

        <div
          v-if="rt && hasResults"
          class="flex-1 basis-3/5 min-h-0 flex flex-col"
          data-testid="console-results"
        >
          <StaleDataStrip
            v-if="stale && hasResults"
            testid="console-stale"
            reconnect-only
            :reconnecting="reconnecting"
            @reconnect="onStaleReconnect"
          />
          <ConsoleResultStrip
            :tab-id="tab.id"
            @expand-all="onExpandAllResults"
            @collapse-all="onCollapseAllResults"
          />
          <SearchToolbar
            v-if="rt.searchOpen && !activeResultIsPlan"
            :tab-id="tab.id"
            :is-open="() => !!consoleViewStore.runtime[tab.id]?.searchOpen"
            testid-prefix="console-"
            kind="rows"
            :api="pageSearchApi"
            @go-to-match="onGoToMatch"
            @close="onCloseSearch"
          />
          <Alert
            strip
            v-if="activeResultTruncated?.truncated"
            variant="warn"
            data-testid="console-result-truncated"
          >
            <AlertDescription>
              {{ t('studio.console.result.truncated.body', { count: activeResultTruncated.rows }) }}
            </AlertDescription>
          </Alert>
          <div class="flex-1 min-h-0">
            <ExplainResultView v-if="rt.activeKey && activeResultIsPlan" :page-key="rt.activeKey" />
            <ConsoleResultGrid
              v-else-if="rt.activeKey"
              ref="resultGridRef"
              :page-key="rt.activeKey"
              :tab-id="tab.id"
              :connection-id="tab.connectionId"
              :path="tab.path"
            />
          </div>
        </div>
      </div>
      <CellEditorDock :tab-id="tab.id" readonly />
    </div>
  </div>
</template>
