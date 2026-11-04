<script setup lang="ts">
import { isGrpcDirty, toSavedGrpcRequest } from '@kira/api-core';
import { grpcRequestTitle } from '@shared/domain/grpc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RunState from '@theme/RunState.vue';
import AutocompleteField from '@workbench/components/AutocompleteField.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { templateToken } from '@workbench/editor/fieldCompletion';
import { registerCommand } from '@workbench/shortcuts/commands';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onMounted, onUnmounted, ref, useTemplateRef } from 'vue';
import EnvironmentSelect from '../../api/EnvironmentSelect.vue';
import { useSavedGrpcRequest, useVariableRows } from '../../api/state/apiQueries';
import { useCollectionsStore } from '../../api/state/collections';
import { useSaveRequestDialogStore } from '../../api/state/saveRequestDialog';
import {
  variableCompletionSource,
  variableHoverSource,
  variableSupport,
} from '../../api/state/variableCompletion';
import { mergeVariableRows } from '../../api/state/variables';
import { patchGrpcRequestTabState } from '../../api/tabs';
import VariablesOverviewPanel from '../../api/VariablesOverviewPanel.vue';
import { beautifyJson } from '../../beautify';
import MonacoHost from '../../editor/MonacoHost.vue';
import type { GrpcRequestTabRecord } from '../../state/tabDomain';
import { INLINE_CHARS } from '../../workers/parse/client';
import { useParseWorker } from '../../workers/parse/useParseWorker';
import { reasonText } from '../shared/reasonText';
import { unresolvedOf, unresolvedTooltipOf } from '../shared/request/unresolved';
import { useRequestChrome } from '../shared/request/useRequestChrome';
import { useRequestTabSave } from '../shared/request/useRequestTabSave';
import GrpcMetadataTable from './GrpcMetadataTable.vue';
import ResponsePane from './ResponsePane.vue';
import { resolveGrpcTabState } from './resolve';
import SchemaBrowser from './SchemaBrowser.vue';
import { useGrpcSchema } from './schemaQuery';
import { findMethod, useGrpcRequestViewStore } from './state';

// MainView.vue keys this component by tab.id — same discipline as every other *View.vue.
const props = defineProps<{ tab: GrpcRequestTabRecord }>();

const collectionsStore = useCollectionsStore();
const saveRequestDialogStore = useSaveRequestDialogStore();
const grpcRequestViewStore = useGrpcRequestViewStore();

const rt = computed(() => grpcRequestViewStore.runtime[props.tab.id]);
const running = computed(() => rt.value?.status === 'running');
const title = computed(() => grpcRequestTitle(props.tab.state));

// Call disables until findMethod resolves the tab's method: before the schema loads, call() would
// default a streaming method to unary.
const methodResolved = computed(() => {
  return !!findMethod(schema.value, props.tab.state.service, props.tab.state.method);
});

const { railColor, runState, incognito, toggleIncognito, envId } = useRequestChrome(
  () => props.tab,
);

const TLS_OPTIONS = computed(() => [
  { value: 'tls' as const, label: t('studio.common.tls.label'), testid: 'grpc-tls-tls' },
  {
    value: 'plaintext' as const,
    label: t('studio.grpc.tls.plaintext.option'),
    testid: 'grpc-tls-plaintext',
  },
]);
function setTlsMode(mode: 'plaintext' | 'tls'): void {
  patchGrpcRequestTabState(props.tab.id, { tlsMode: mode });
}

function onTargetInput(value: string): void {
  patchGrpcRequestTabState(props.tab.id, { target: value });
}

// Fast-path method picker over the resolved schema; SchemaBrowser has the grouped list.
const methodOptions = computed(() => {
  if (!schema.value) return [];
  const out: { value: string; label: string }[] = [];
  for (const svc of schema.value.services) {
    for (const m of svc.methods) {
      const badge = m.serverStreaming || m.clientStreaming ? ' (stream)' : '';
      out.push({ value: `${svc.fullName}|${m.name}`, label: `${svc.fullName}/${m.name}${badge}` });
    }
  }
  return out;
});
const selectedMethodValue = computed(() => `${props.tab.state.service}|${props.tab.state.method}`);
function onMethodSelect(rawValue: unknown): void {
  const [service, method] = String(rawValue).split('|');
  const m = findMethod(schema.value, service, method);
  patchGrpcRequestTabState(props.tab.id, {
    service,
    method,
    message: m?.requestTemplate ?? props.tab.state.message,
  });
}

const {
  schema,
  loading: schemaLoading,
  error: schemaError,
  reload: reloadSchema,
} = useGrpcSchema(() => props.tab);

const savedQuery = useSavedGrpcRequest(() => props.tab.state.itemId);
const saved = computed(() => savedQuery.data.value ?? null);
const dirty = computed(() => isGrpcDirty(props.tab.state, saved.value));
const canSave = computed(() => props.tab.state.itemId !== null && saved.value !== null);

const unresolved = computed(
  () => props.tab.state.itemId !== null && savedQuery.data.value === undefined,
);

const { onSave } = useRequestTabSave({
  tabId: () => props.tab.id,
  incognito: () => incognito.value,
  itemId: () => props.tab.state.itemId,
  saved: () => saved.value,
  unresolved: () => unresolved.value,
  name: () => props.tab.state.name || title.value,
  toSaved: () => toSavedGrpcRequest(props.tab.state),
  save: (itemId, name, body) => collectionsStore.saveGrpcRequest(itemId, name, body),
  openSaveDialog: (tabId, name, body) =>
    saveRequestDialogStore.openSaveGrpcDialog(tabId, name, body),
});

function onCall(): void {
  void grpcRequestViewStore.call(props.tab.id);
}

function onStop(): void {
  grpcRequestViewStore.stop(props.tab.id);
}

const collectionId = computed(() => collectionsStore.collectionIdFor(props.tab.state));
const colRows = useVariableRows('collection', collectionId);
const envRows = useVariableRows('environment', envId);
const variables = computed(() =>
  variableSupport(colRows.data.value ?? [], envRows.data.value ?? []),
);

const unresolvedRefs = computed(() => {
  const { values, secretNames } = mergeVariableRows(
    colRows.data.value ?? [],
    envRows.data.value ?? [],
  );
  return unresolvedOf(resolveGrpcTabState(props.tab.state, values, secretNames).refs);
});
const saveTooltip = computed(() => {
  if (incognito.value) return t('studio.api.common.save.incognito.tooltip');
  if (unresolved.value) return t('studio.api.common.save.checking.tooltip');
  return t(
    canSave.value ? 'studio.api.saveRequest.title' : 'studio.api.common.save.toCollection.tooltip',
  );
});
const unresolvedTooltip = computed(() => unresolvedTooltipOf(unresolvedRefs.value));

const REQUEST_PANE_OPTIONS = computed(() => [
  {
    value: 'message' as const,
    label: t('common.message.label'),
    testid: 'grpc-request-pane-message',
  },
  {
    value: 'metadata' as const,
    label: t('studio.grpc.pane.metadata.option'),
    testid: 'grpc-request-pane-metadata',
  },
  {
    value: 'schema' as const,
    label: t('studio.common.nodeKind.schema.label'),
    testid: 'grpc-request-pane-schema',
  },
]);
function setRequestPane(pane: 'message' | 'metadata' | 'schema'): void {
  patchGrpcRequestTabState(props.tab.id, { requestPane: pane });
}

// Metadata filter: per-tab runtime (P375), a lens rather than tab state.
const fieldFilterOpen = computed(() => rt.value?.fieldFilterOpen ?? false);
const fieldFilterQuery = computed({
  get: () => rt.value?.fieldFilterQuery ?? '',
  set: (v: string) => grpcRequestViewStore.setFieldFilterQuery(props.tab.id, v),
});
function toggleFieldFilter(): void {
  const open = !fieldFilterOpen.value;
  grpcRequestViewStore.setFieldFilter(props.tab.id, open, open ? fieldFilterQuery.value : '');
}

function toggleFieldDescriptions(): void {
  patchGrpcRequestTabState(props.tab.id, {
    fieldDescriptions: !props.tab.state.fieldDescriptions,
  });
}

const overviewOpen = ref(false);
const overviewAnchorRef = ref<HTMLElement | null>(null);

const beautifyError = ref<string | null>(null);
function onMessageInput(value: string): void {
  patchGrpcRequestTabState(props.tab.id, { message: value });
  beautifyError.value = null;
}

const DEFAULT_REQUEST_PANE_HEIGHT = 260;
const requestPane = useLiveSize(
  () => props.tab.state.requestPaneHeight || DEFAULT_REQUEST_PANE_HEIGHT,
  (size) => patchGrpcRequestTabState(props.tab.id, { requestPaneHeight: size }),
);

const parse = useParseWorker();
async function onBeautify(): Promise<void> {
  const source = props.tab.state.message;
  const result =
    source.length <= INLINE_CHARS
      ? beautifyJson(source, 'indented')
      : await parse.run('json.beautify', { text: source, mode: 'indented' }).catch(() => null);
  // Null: the view unmounted mid-run. Otherwise the message may have moved on while the worker ran.
  if (!result || props.tab.state.message !== source) return;
  if (result.ok) {
    patchGrpcRequestTabState(props.tab.id, { message: result.text });
    beautifyError.value = null;
  } else {
    beautifyError.value = result.reason
      ? reasonText(result.reason)
      : t('studio.grpc.message.beautifyFailed.error');
  }
}

const rootEl = useTemplateRef<HTMLElement>('rootEl');
useShortcut(rootEl, ['form.save'], onSave);

let unregisterCommands: Array<() => void> = [];
onMounted(() => {
  unregisterCommands = [
    registerCommand('view.run', onCall),
    registerCommand('view.refresh', onCall),
    registerCommand('view.format', onBeautify),
    registerCommand('api.save', onSave),
  ];
});
onUnmounted(() => {
  for (const off of unregisterCommands) off();
});
</script>

<template>
  <div ref="rootEl" class="flex h-full min-h-0 flex-col" data-testid="grpc-request-view">
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span class="size-4 flex items-center justify-center shrink-0"
        ><CodiconIcon name="symbol-interface" :size="13" /></span
      >
      <span class="text-kira-md text-fg truncate" data-testid="grpc-request-target">{{
        title
      }}</span>
      <Badge
        data
        v-if="tab.state.service && tab.state.method"
        variant="info"
        data-testid="grpc-method-chip"
      >
        {{ tab.state.service }}/{{ tab.state.method }}
      </Badge>
      <Tooltip v-if="dirty">
        <TooltipTrigger as-child>
          <span
            class="text-warn text-kira-lg leading-none"
            role="img"
            data-testid="grpc-dirty"
            :aria-label="t('common.state.unsavedChanges.status')"
            >•</span
          >
        </TooltipTrigger>
        <TooltipContent>{{ t('common.state.unsavedChanges.status') }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="unresolvedRefs.length > 0">
        <TooltipTrigger as-child>
          <Badge variant="warn" data-testid="grpc-unresolved-chip">{{
            t('common.unresolved.label', { count: unresolvedRefs.length })
          }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{ unresolvedTooltip }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="incognito">
        <TooltipTrigger as-child>
          <Badge variant="chip" data-testid="grpc-incognito-chip">{{
            t('studio.tabs.incognito.label')
          }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.api.common.incognitoChip.tooltip') }}</TooltipContent>
      </Tooltip>
      <span class="ml-auto flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger as-child>
            <TooltipDisabledTrigger>
              <Button
                variant="toolbar"
                size="kira"
                data-testid="grpc-save"
                :disabled="incognito || unresolved || (canSave && !dirty)"
                @click="onSave"
              >
                <CodiconIcon name="save" :size="13" />
                {{ t('common.action.save.label') }}
              </Button>
            </TooltipDisabledTrigger>
          </TooltipTrigger>
          <TooltipContent>{{ saveTooltip }}</TooltipContent>
        </Tooltip>
      </span>
    </ViewToolbar>
    <div :class="colorMarkClass('band', railColor)" />
    <ViewToolbar>
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          :icon="ICON.refresh"
          :label="t('common.action.refresh.label')"
          data-testid="grpc-request-refresh"
          @click="onCall"
        />
        <TooltipIconButton
          :icon="ICON.stop"
          :label="t('common.action.stop.label')"
          disabled-trigger
          :class="{ 'is-live': running }"
          :disabled="!running"
          data-testid="grpc-request-stop"
          @click="onStop"
        />
      </div>
      <div class="grpc-target-field min-w-0 flex-1">
        <AutocompleteField
          class="w-full"
          :model-value="tab.state.target"
          :placeholder="t('studio.grpc.target.example')"
          data-testid="grpc-target"
          :candidates="variables.candidates"
          :token-at="templateToken"
          :range-highlights="variables.rangeHighlights"
          :hover-at="variables.hoverAt"
          @update:model-value="onTargetInput"
          @enter="onCall"
        />
      </div>
      <SecondaryTabs
        variant="segmented"
        :model-value="tab.state.tlsMode"
        :items="TLS_OPTIONS"
        data-testid="grpc-tls-toggle"
        @update:model-value="(v) => setTlsMode(v as 'plaintext' | 'tls')"
      />
      <div class="min-w-0 flex-1">
        <NativeSelect
          variant="bordered"
          class="w-full"
          data-testid="grpc-method-select"
          :model-value="selectedMethodValue"
          :disabled="methodOptions.length === 0"
          @update:model-value="onMethodSelect"
        >
          <option value="" disabled>{{ t('studio.grpc.method.choose.placeholder') }}</option>
          <option v-for="opt in methodOptions" :key="opt.value" :value="opt.value">{{
            opt.label
          }}</option>
        </NativeSelect>
      </div>
      <Tooltip>
        <TooltipTrigger as-child>
          <TooltipDisabledTrigger>
            <Button
              variant="toolbar-primary"
              size="kira"
              data-testid="grpc-call"
              :pending="running"
              :disabled="!tab.state.service || !tab.state.method || !methodResolved"
              @click="onCall"
            >
              <CodiconIcon :name="ICON.run" :size="13" />
              {{ t('studio.grpc.call.label') }}
            </Button>
          </TooltipDisabledTrigger>
        </TooltipTrigger>
        <TooltipContent>{{
          !running && tab.state.service && tab.state.method && !methodResolved
            ? t('studio.grpc.call.waitingSchema.tooltip')
            : t('studio.grpc.call.label')
        }}</TooltipContent>
      </Tooltip>
      <span class="ml-auto" />
      <RunState :state="runState" />
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          icon="eye-closed"
          :label="
            incognito
              ? t('studio.api.common.incognitoOff.tooltip')
              : t('studio.api.common.incognitoOn.tooltip')
          "
          :aria-label="t('studio.tabs.incognito.label')"
          :pressed="incognito"
          data-testid="grpc-incognito-toggle"
          @click="toggleIncognito"
        />
      </div>
    </ViewToolbar>

    <ViewToolbar>
      <SecondaryTabs
        :model-value="tab.state.requestPane"
        :items="REQUEST_PANE_OPTIONS"
        data-testid="grpc-request-pane-toggle"
        @update:model-value="(v) => setRequestPane(v as 'message' | 'metadata' | 'schema')"
      />
      <TooltipIconButton
        v-if="tab.state.requestPane === 'message'"
        icon="expand-all"
        :label="t('studio.api.common.beautify.label')"
        data-testid="grpc-beautify"
        @click="onBeautify"
      />
      <TooltipIconButton
        v-if="tab.state.requestPane === 'metadata'"
        :icon="ICON.search"
        :label="t('common.action.filter.label')"
        :pressed="fieldFilterOpen"
        data-testid="grpc-field-filter-toggle"
        @click="toggleFieldFilter"
      />
      <TooltipIconButton
        v-if="tab.state.requestPane === 'metadata'"
        icon="note"
        :label="
          tab.state.fieldDescriptions
            ? t('studio.api.common.descriptionsHide.tooltip')
            : t('studio.api.common.descriptionsShow.tooltip')
        "
        :aria-label="t('studio.api.common.descriptions.aria')"
        :pressed="tab.state.fieldDescriptions"
        data-testid="grpc-field-descriptions-toggle"
        @click="toggleFieldDescriptions"
      />
      <Popover :open="overviewOpen" @update:open="overviewOpen = $event">
        <div ref="overviewAnchorRef" class="relative flex">
          <TooltipIconButton
            :icon="ICON.variables"
            :label="t('common.module.variables.label')"
            :pressed="overviewOpen"
            data-testid="grpc-variables-overview-toggle"
            @click="overviewOpen = !overviewOpen"
          />
          <PopoverAnchor :reference="overviewAnchorRef ?? undefined" />
        </div>
        <VariablesOverviewPanel
          v-if="overviewOpen"
          :collection-id="collectionId"
          :environment-id="envId"
          :can-edit="!incognito"
          @close="overviewOpen = false"
        />
      </Popover>
      <EnvironmentSelect :tab-id="tab.id" />
    </ViewToolbar>

    <div class="flex flex-1 min-h-0 flex-col">
      <div
        class="request-pane flex min-h-0 flex-col overflow-hidden"
        data-testid="grpc-request-pane"
        :style="{ flex: `0 1 ${requestPane.size.value}px` }"
      >
        <Alert
          strip
          v-if="beautifyError && tab.state.requestPane === 'message'"
          variant="error"
          data-testid="grpc-beautify-error"
        >
          <AlertDescription>{{ beautifyError }}</AlertDescription>
        </Alert>
        <MonacoHost
          v-if="tab.state.requestPane === 'message'"
          :doc="tab.state.message"
          language="json"
          :range-highlights="variables.rangeHighlights"
          :hover-source="variableHoverSource(variables.hoverInfo)"
          autocomplete
          :completion-sources="[variableCompletionSource(variables.candidates)]"
          auto-close-brackets
          data-testid="grpc-message-editor"
          @update:doc="onMessageInput"
        />
        <template v-else-if="tab.state.requestPane === 'metadata'">
          <SearchField
            v-if="fieldFilterOpen"
            v-model="fieldFilterQuery"
            :placeholder="t('common.action.filter.label')"
            data-testid="grpc-field-filter"
          />
          <GrpcMetadataTable
            :tab="tab"
            :filter-query="fieldFilterQuery"
            :variables="variables"
            :show-descriptions="tab.state.fieldDescriptions"
          />
        </template>
        <SchemaBrowser
          v-else
          :tab="tab"
          :schema="schema"
          :loading="schemaLoading"
          :error="schemaError"
          @reload="reloadSchema"
        />
      </div>

      <ResizeHandle
        class="request-splitter"
        :label="t('studio.api.common.resizeRequest.aria')"
        axis="y"
        line
        :min="120"
        :max="800"
        v-bind="requestPane.bind"
      />

      <div class="min-h-0 flex-1" data-testid="grpc-response-pane-slot">
        <ResponsePane :tab="tab" />
      </div>
    </div>
  </div>
</template>
