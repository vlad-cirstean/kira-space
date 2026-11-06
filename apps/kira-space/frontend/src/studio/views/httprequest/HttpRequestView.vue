<script setup lang="ts">
import {
  canEditAsRaw,
  defaultContentTypeFor,
  generateRawRequest,
  httpRequestTitle,
  isDirty,
  looksLikeCurlCommand,
  parseQuery,
  splitUrl,
  toSavedRequest,
} from '@kira/api-core';
import { type HttpMethod, type HttpRequestPane, httpMethodToken } from '@shared/domain/http';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
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
import { methodTextClass } from '@theme/methodColor';
import RunState from '@theme/RunState.vue';
import { refDebounced } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import AutocompleteField from '@workbench/components/AutocompleteField.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { templateToken } from '@workbench/editor/fieldCompletion';
import { registerCommand } from '@workbench/shortcuts/commands';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onMounted, onUnmounted, ref, useTemplateRef } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import type { HttpRequestTabRecord } from '../../../state/tabDomain';
import EnvironmentSelect from '../../api/EnvironmentSelect.vue';
import MethodSelect from '../../api/MethodSelect.vue';
import { useSavedRequest, useVariableRows } from '../../api/state/apiQueries';
import { useCollectionsStore } from '../../api/state/collections';
import { applyCurlToTab, useCopyAsCurlStore } from '../../api/state/curl';
import { useEditRawStore } from '../../api/state/raw';
import { useSaveRequestDialogStore } from '../../api/state/saveRequestDialog';
import { variableSupport } from '../../api/state/variableCompletion';
import { mergeVariableRows } from '../../api/state/variables';
import { patchHttpRequestTabState } from '../../api/tabs';
import VariablesOverviewPanel from '../../api/VariablesOverviewPanel.vue';
import ResponseFindBar from '../shared/ResponseFindBar.vue';
import { unresolvedOf, unresolvedTooltipOf } from '../shared/request/unresolved';
import { useRequestChrome } from '../shared/request/useRequestChrome';
import { useRequestTabSave } from '../shared/request/useRequestTabSave';
import { bodyTabItem } from './bodyTabItem';
import CookiesPane from './CookiesPane.vue';
import { useJarCookies } from './cookies';
import QueryParamsTable from './QueryParamsTable.vue';
import RequestBodyPane from './RequestBodyPane.vue';
import RequestHeadersTable from './RequestHeadersTable.vue';
import RequestSettingsPane from './RequestSettingsPane.vue';
import ResponsePane from './ResponsePane.vue';
import { resolveForExport, resolveTabState, useHttpRequestViewStore } from './state';
import { type FindBarHandle, type RequestBodyHandle, useRequestFind } from './useRequestFind';

// MainView.vue keys this component by tab.id — same discipline as every other *View.vue.
const props = defineProps<{ tab: HttpRequestTabRecord }>();

const editRawStore = useEditRawStore();
const collectionsStore = useCollectionsStore();
const saveRequestDialogStore = useSaveRequestDialogStore();
const copyAsCurlStore = useCopyAsCurlStore();
const settingsStore = useSettingsStore();
const httpRequestViewStore = useHttpRequestViewStore();

const rt = computed(() => httpRequestViewStore.runtime[props.tab.id]);
const running = computed(() => rt.value?.status === 'running');

const title = computed(() => httpRequestTitle(props.tab.state));

const { railColor, runState, incognito, toggleIncognito, envId } = useRequestChrome(
  () => props.tab,
);

const methodToken = computed(() => httpMethodToken(props.tab.state.method));

function onMethodChange(method: HttpMethod): void {
  patchHttpRequestTabState(props.tab.id, { method });
}

const requestBodyRef = useTemplateRef<RequestBodyHandle>('requestBodyRef');
const requestFindBarRef = useTemplateRef<FindBarHandle>('requestFindBarRef');
const {
  requestFindOpen,
  requestFindTargets,
  requestFindHighlights,
  toggleRequestFind,
  closeRequestFind,
} = useRequestFind(requestBodyRef, requestFindBarRef);

function onUrlInput(value: string): void {
  patchHttpRequestTabState(props.tab.id, { url: value });
}

function onUrlPaste(text: string, e: ClipboardEvent): void {
  if (!looksLikeCurlCommand(text)) return;
  if (!applyCurlToTab(props.tab.id, text)) return;
  e.preventDefault();
}

const savedQuery = useSavedRequest(() => props.tab.state.itemId);
const saved = computed(() => savedQuery.data.value ?? null);
const dirty = computed(() => isDirty(props.tab.state, saved.value));
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
  toSaved: () => toSavedRequest(props.tab.state),
  save: (itemId, name, body) => collectionsStore.saveRequest(itemId, name, body),
  openSaveDialog: (tabId, name, body) => saveRequestDialogStore.openSaveDialog(tabId, name, body),
});

function onSend(): void {
  void httpRequestViewStore.send(props.tab.id);
}

async function onCopyAsCurl(): Promise<void> {
  const resolution = await resolveForExport(props.tab.id);
  if (!resolution) return;
  copyAsCurlStore.openCopyAsCurlDialog(
    resolution.method,
    resolution.resolved,
    resolution.deferredNames,
    defaultContentTypeFor(props.tab.state.bodyMode, props.tab.state.codeLanguage),
    collectionId.value,
    envId.value,
  );
}

const canEditRaw = computed(() => canEditAsRaw(props.tab.state.bodyMode));
const editRawTooltip = computed(() =>
  canEditRaw.value ? t('studio.api.editRaw.title') : t('studio.http.editRaw.unavailable.tooltip'),
);

function onEditRaw(): void {
  if (!canEditRaw.value) return;
  const initialText = generateRawRequest(
    props.tab.state,
    defaultContentTypeFor(props.tab.state.bodyMode, props.tab.state.codeLanguage),
  );
  editRawStore.openEditRawDialog(
    props.tab.id,
    initialText,
    props.tab.state.bodyMode,
    props.tab.state.url,
  );
}

const collectionId = computed(() => collectionsStore.collectionIdFor(props.tab.state));
const colRows = useVariableRows('collection', collectionId);
const envRows = useVariableRows('environment', envId);
const variables = computed(() =>
  variableSupport(colRows.data.value ?? [], envRows.data.value ?? []),
);

const resolvedTab = computed(() => {
  const { values, secretNames } = mergeVariableRows(
    colRows.data.value ?? [],
    envRows.data.value ?? [],
  );
  return resolveTabState(props.tab.state, values, secretNames);
});
const unresolvedRefs = computed(() => unresolvedOf(resolvedTab.value.refs));
const saveTooltip = computed(() => {
  if (incognito.value) return t('studio.api.common.save.incognito.tooltip');
  if (unresolved.value) return t('studio.api.common.save.checking.tooltip');
  return t(
    canSave.value ? 'studio.api.saveRequest.title' : 'studio.api.common.save.toCollection.tooltip',
  );
});
const unresolvedTooltip = computed(() => unresolvedTooltipOf(unresolvedRefs.value));

function onStop(): void {
  httpRequestViewStore.stop(props.tab.id);
}

const paramsCount = computed(() => parseQuery(splitUrl(props.tab.state.url).query).length);
const headersCount = computed(() => props.tab.state.headers.filter((h) => h.enabled).length);

const settingsOverrideCount = computed(
  () => Object.values(props.tab.state.settings).filter((v) => v !== null).length,
);

const requestCookiesCount = computed(() => jarCookies.data.value?.length ?? 0);

const countOrUndefined = (n: number): number | undefined => (n > 0 ? n : undefined);
const REQUEST_PANE_OPTIONS = computed(() => [
  {
    value: 'params' as const,
    label: t('studio.http.pane.params.label'),
    count: countOrUndefined(paramsCount.value),
    testid: 'http-request-pane-params',
  },
  {
    value: 'headers' as const,
    label: t('studio.http.pane.headers.label'),
    count: countOrUndefined(headersCount.value),
    testid: 'http-request-pane-headers',
  },
  { ...bodyTabItem(props.tab.state), value: 'body' as const, testid: 'http-request-pane-body' },
  {
    value: 'settings' as const,
    label: t('common.module.settings.label'),
    count: countOrUndefined(settingsOverrideCount.value),
    testid: 'http-request-pane-settings',
  },
  {
    value: 'cookies' as const,
    label: t('studio.http.pane.cookies.label'),
    count: countOrUndefined(requestCookiesCount.value),
    testid: 'http-request-pane-cookies',
  },
]);

function setRequestPane(pane: HttpRequestPane): void {
  patchHttpRequestTabState(props.tab.id, { requestPane: pane });
}

const fieldFilterOpen = computed(() => rt.value?.fieldFilterOpen ?? false);
const fieldFilterQuery = computed({
  get: () => rt.value?.fieldFilterQuery ?? '',
  set: (v: string) => httpRequestViewStore.setFieldFilterQuery(props.tab.id, v),
});

const overviewOpen = ref(false);
const overviewAnchorRef = ref<HTMLElement | null>(null);
const showFieldFilterToggle = computed(
  () =>
    props.tab.state.requestPane === 'params' ||
    props.tab.state.requestPane === 'headers' ||
    (props.tab.state.requestPane === 'body' &&
      (props.tab.state.bodyMode === 'urlencoded' || props.tab.state.bodyMode === 'formdata')),
);

const effectiveSslVerify = computed(
  () => props.tab.state.settings.sslVerify ?? settingsStore.api.sslVerify,
);

const effectiveDisableCookieJar = computed(
  () => props.tab.state.settings.disableCookieJar ?? settingsStore.api.disableCookieJar,
);
const resolvedCookiesUrl = computed(() => resolvedTab.value.url);
const cookiesUrl = refDebounced(resolvedCookiesUrl, TIMING.preview);
const jarCookies = useJarCookies(cookiesUrl, () => !effectiveDisableCookieJar.value);
function toggleFieldFilter(): void {
  const open = !fieldFilterOpen.value;
  httpRequestViewStore.setFieldFilter(props.tab.id, open, open ? fieldFilterQuery.value : '');
}

function toggleFieldDescriptions(): void {
  patchHttpRequestTabState(props.tab.id, { fieldDescriptions: !props.tab.state.fieldDescriptions });
}

const DEFAULT_REQUEST_PANE_HEIGHT = 260;
const requestPane = useLiveSize(
  () => props.tab.state.requestPaneHeight || DEFAULT_REQUEST_PANE_HEIGHT,
  (size) => patchHttpRequestTabState(props.tab.id, { requestPaneHeight: size }),
);

const rootEl = useTemplateRef<HTMLElement>('rootEl');
useShortcut(rootEl, ['form.save'], onSave);

let unregisterCommands: Array<() => void> = [];
onMounted(() => {
  unregisterCommands = [
    registerCommand('view.run', onSend),
    registerCommand('view.refresh', onSend),
    registerCommand('api.save', onSave),
    registerCommand('api.copyAsCurl', onCopyAsCurl),
    registerCommand('api.editRaw', onEditRaw),
  ];
});
onUnmounted(() => {
  for (const off of unregisterCommands) off();
});
</script>

<template>
  <div ref="rootEl" class="flex h-full min-h-0 flex-col" data-testid="http-request-view">
    <ViewToolbar data-testid="view-head">
      <span
        v-if="railColor !== undefined"
        :class="colorMarkClass('dot', railColor)"
        data-testid="conn-dot"
      />
      <span class="size-4 flex items-center justify-center shrink-0"
        ><CodiconIcon name="globe" :size="13" /></span
      >
      <span class="text-kira-md text-fg truncate" data-testid="http-request-target">{{
        title
      }}</span>
      <Badge
        data
        variant="chip"
        :class="methodTextClass(methodToken)"
        data-testid="http-method-chip"
        >{{
          tab.state.method
        }}</Badge
      >
      <Tooltip v-if="dirty">
        <TooltipTrigger as-child>
          <span
            class="text-warn text-kira-lg leading-none"
            role="img"
            data-testid="http-dirty"
            :aria-label="t('common.state.unsavedChanges.status')"
            >•</span
          >
        </TooltipTrigger>
        <TooltipContent>{{ t('common.state.unsavedChanges.status') }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="unresolvedRefs.length > 0">
        <TooltipTrigger as-child>
          <Badge variant="warn" data-testid="http-unresolved-chip">{{
            t('common.unresolved.label', { count: unresolvedRefs.length })
          }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{ unresolvedTooltip }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="incognito">
        <TooltipTrigger as-child>
          <Badge variant="chip" data-testid="http-incognito-chip">{{
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
                data-testid="http-save"
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
    <div :class="colorMarkClass('band', railColor)" data-testid="toolbar-rail" />
    <ViewToolbar>
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          :icon="ICON.refresh"
          :label="t('common.action.refresh.label')"
          data-testid="http-request-refresh"
          @click="onSend"
        />
        <TooltipIconButton
          :icon="ICON.stop"
          :label="t('common.action.stop.label')"
          disabled-trigger
          :class="{ 'is-live': running }"
          :disabled="!running"
          data-testid="http-request-stop"
          @click="onStop"
        />
      </div>
      <MethodSelect
        :model-value="tab.state.method"
        testid="http-method-select"
        @update:model-value="onMethodChange"
      />
      <div class="url-field min-w-0 flex-1">
        <AutocompleteField
          class="w-full"
          :model-value="tab.state.url"
          :placeholder="t('studio.http.url.example')"
          data-testid="http-url"
          :candidates="variables.candidates"
          :token-at="templateToken"
          :range-highlights="variables.rangeHighlights"
          :hover-at="variables.hoverAt"
          @update:model-value="onUrlInput"
          @paste-text="onUrlPaste"
          @enter="onSend"
        />
      </div>
      <Tooltip v-if="!effectiveSslVerify">
        <TooltipTrigger as-child>
          <Badge variant="warn" data-testid="http-ssl-verify-off-chip">
            <CodiconIcon name="unverified" :size="13" />
          </Badge>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.http.sslOff.tooltip') }}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger as-child>
          <TooltipDisabledTrigger>
            <Button
              variant="dialog-primary"
              size="kira"
              data-testid="http-send"
              :pending="running"
              @click="onSend"
            >
              <CodiconIcon :name="ICON.run" :size="13" />
              {{ t('common.action.send.label') }}
            </Button>
          </TooltipDisabledTrigger>
        </TooltipTrigger>
        <TooltipContent>{{ t('common.action.send.label') }}</TooltipContent>
      </Tooltip>
      <span class="ml-auto" />
      <RunState :state="runState" />
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          icon="terminal"
          :label="t('studio.api.copyAsCurl.title')"
          :aria-label="t('studio.api.copyAsCurl.title')"
          data-testid="http-copy-as-curl"
          @click="onCopyAsCurl"
        />
        <TooltipIconButton
          icon="code"
          :label="editRawTooltip"
          :aria-label="t('studio.api.editRaw.title')"
          disabled-trigger
          :disabled="!canEditRaw"
          data-testid="http-edit-raw"
          @click="onEditRaw"
        />
        <TooltipIconButton
          icon="eye-closed"
          :label="
            incognito
              ? t('studio.api.common.incognitoOff.tooltip')
              : t('studio.api.common.incognitoOn.tooltip')
          "
          :aria-label="t('studio.tabs.incognito.label')"
          :pressed="incognito"
          data-testid="http-incognito-toggle"
          @click="toggleIncognito"
        />
      </div>
    </ViewToolbar>

    <ViewToolbar>
      <SecondaryTabs
        :model-value="tab.state.requestPane"
        :items="REQUEST_PANE_OPTIONS"
        data-testid="http-request-pane-toggle"
        @update:model-value="(v) => setRequestPane(v as HttpRequestPane)"
      />
      <TooltipIconButton
        v-if="showFieldFilterToggle"
        :icon="ICON.search"
        :label="t('common.action.filter.label')"
        :pressed="fieldFilterOpen"
        data-testid="http-field-filter-toggle"
        @click="toggleFieldFilter"
      />
      <TooltipIconButton
        v-if="showFieldFilterToggle"
        icon="note"
        :label="
          tab.state.fieldDescriptions
            ? t('studio.api.common.descriptionsHide.tooltip')
            : t('studio.api.common.descriptionsShow.tooltip')
        "
        :aria-label="t('studio.api.common.descriptions.aria')"
        :pressed="tab.state.fieldDescriptions"
        data-testid="http-field-descriptions-toggle"
        @click="toggleFieldDescriptions"
      />
      <TooltipIconButton
        :icon="ICON.search"
        :label="t('studio.http.request.find.label')"
        :aria-label="t('studio.http.request.find.label')"
        :pressed="requestFindOpen"
        data-testid="http-request-find-toggle"
        @click="toggleRequestFind"
      />
      <Popover :open="overviewOpen" @update:open="overviewOpen = $event">
        <div ref="overviewAnchorRef" class="relative flex">
          <TooltipIconButton
            :icon="ICON.variables"
            :label="t('common.module.variables.label')"
            :pressed="overviewOpen"
            data-testid="http-variables-overview-toggle"
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

    <ResponseFindBar
      v-if="requestFindOpen"
      ref="requestFindBarRef"
      :targets="requestFindTargets"
      @close="closeRequestFind"
    />

    <div class="flex flex-1 min-h-0 flex-col">
      <div
        class="request-pane flex min-h-0 flex-col overflow-hidden"
        data-testid="http-request-pane"
        :style="{ flex: `0 1 ${requestPane.size.value}px` }"
      >
        <SearchField
          v-if="fieldFilterOpen && showFieldFilterToggle"
          v-model="fieldFilterQuery"
          :placeholder="t('common.action.filter.label')"
          data-testid="http-field-filter"
        />
        <QueryParamsTable
          v-if="tab.state.requestPane === 'params'"
          :tab="tab"
          :variables="variables"
          :filter-query="fieldFilterQuery"
          :show-descriptions="tab.state.fieldDescriptions"
        />
        <RequestHeadersTable
          v-else-if="tab.state.requestPane === 'headers'"
          :tab="tab"
          :variables="variables"
          :filter-query="fieldFilterQuery"
          :show-descriptions="tab.state.fieldDescriptions"
        />
        <RequestSettingsPane v-else-if="tab.state.requestPane === 'settings'" :tab="tab" />
        <CookiesPane
          v-else-if="tab.state.requestPane === 'cookies'"
          mode="request"
          :tab-id="tab.id"
          :url="cookiesUrl"
          :cookies="jarCookies.data.value"
          :error="jarCookies.error.value"
          :disable-cookie-jar="effectiveDisableCookieJar"
          @refresh="jarCookies.refetch()"
        />
        <RequestBodyPane
          v-else
          ref="requestBodyRef"
          :tab="tab"
          :variables="variables"
          :find-highlights="requestFindHighlights"
          :filter-query="fieldFilterQuery"
          :show-descriptions="tab.state.fieldDescriptions"
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

      <div class="min-h-0 flex-1" data-testid="http-response-pane-slot">
        <ResponsePane :tab="tab" />
      </div>
    </div>
  </div>
</template>
