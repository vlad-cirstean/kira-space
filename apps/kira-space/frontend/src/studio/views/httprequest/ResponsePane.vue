<script setup lang="ts">
import { type HttpResponsePane, statusClass, statusHint } from '@shared/domain/http';
import EmptyState from '@theme/components/EmptyState.vue';
import InlineState from '@theme/components/InlineState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { formatBytes, formatClockTime, formatDuration } from '@workbench/behaviour/format';
import { matchesQuery } from '@workbench/behaviour/match';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import type { RangeHighlight } from '@workbench/editor/ranges';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { patchHttpRequestTabState } from '../../api/tabs';
import { DEFAULT_FIND_OPTIONS, type FindOptions, findRanges } from '../../editor/findRanges';
import MonacoHost from '../../editor/MonacoHost.vue';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import ResponseFindBar, {
  type FindBarHost,
  type FindBarTarget,
} from '../shared/ResponseFindBar.vue';
import CookiesPane from './CookiesPane.vue';
import { useHttpHistoryList, useHttpHistoryStore, useHttpHistoryViewing } from './history';
import RawExchangePane from './RawExchangePane.vue';
import ResponseDiffDialog from './ResponseDiffDialog.vue';
import ResponseHistoryList from './ResponseHistoryList.vue';
import { useHttpRequestViewStore } from './state';
import TimelinePane from './TimelinePane.vue';
import { useResponseBody } from './useResponseBody';

const props = defineProps<{ tab: HttpRequestTabRecord }>();
const httpRequestViewStore = useHttpRequestViewStore();
const httpHistoryStore = useHttpHistoryStore();

const rt = computed(() => httpRequestViewStore.runtime[props.tab.id]);
// P175 D4: the list (count only here; the History pane's own observer enables it) and the viewed
// entry's snapshot are TanStack queries — the list's initial fetch and the itemId-change refetch
// (Save as… adopts a scratch tab's history, P8 D14/C5) are the composable's own prefetch.
const { entries: historyEntries } = useHttpHistoryList(() => props.tab);
const { viewing } = useHttpHistoryViewing(() => props.tab.id);

// P8 C6/D12: the dialog mounts only while a compare is in flight — the same "reached only from an
// explicit click" gate that keeps Monaco's chunk unfetched (beyond whatever else on the page already needed it) until then (D13).
const compareIds = ref<[string, string] | null>(null);
function onCompare(ids: [string, string]): void {
  compareIds.value = ids;
}
function closeCompare(): void {
  compareIds.value = null;
}

// The source swap — a selected history entry's response, or the live one, or none. Every
// consumer below (the status chip, the hint, elapsed/bytes, the redirect caption, the truncation
// strip, the headers list, the binary note, prettyFormat, bodyText) reads only this one object.
const response = computed(() => viewing.value?.snapshot.response ?? rt.value?.response ?? null);

const hasHistory = computed(() => historyEntries.value.length > 0);
const historyCount = computed(() => historyEntries.value.length);

// P90 §3.2: sent + received, the Cookies segment's own count badge.
const responseCookiesCount = computed(
  () => (response.value?.sentCookies?.length ?? 0) + (response.value?.receivedCookies?.length ?? 0),
);

const RESPONSE_PANE_OPTIONS = computed(() => [
  { value: 'body' as const, label: t('common.body.label'), testid: 'http-response-pane-body' },
  {
    value: 'headers' as const,
    label: t('studio.http.pane.headers.label'),
    testid: 'http-response-pane-headers',
  },
  {
    value: 'history' as const,
    label: t('common.module.history.label'),
    testid: 'http-response-pane-history',
  },
  // P9 D12/F19: the fourth segment — never on screen at the same time as the body's own Pretty/Raw
  // toggle (gated on responsePane === 'body' below), so the shared "Raw" label never collides.
  { value: 'raw' as const, label: t('common.raw.label'), testid: 'http-response-pane-raw' },
  // P10 D11/F19: the fifth segment — where the time went, per hop.
  {
    value: 'timeline' as const,
    label: t('studio.http.pane.timeline.label'),
    testid: 'http-response-pane-timeline',
  },
]);

// P90 §3.2: the sixth segment, count-badged like REQUEST_PANE_OPTIONS's own Settings/Cookies
// entries — a computed array since the count changes with the response.
const RESPONSE_PANE_OPTIONS_WITH_COOKIES = computed(() => [
  ...RESPONSE_PANE_OPTIONS.value,
  {
    value: 'cookies' as const,
    label: t('studio.http.pane.cookies.label'),
    count: responseCookiesCount.value > 0 ? responseCookiesCount.value : undefined,
    testid: 'http-response-pane-cookies',
  },
]);

// P8 C1: HttpResponsePane, not an inline 'body' | 'headers' literal — the schema is the source of
// truth for the pane vocabulary, so a widened schema (P8 adds 'history', P9 adds 'raw') can never
// desync from this handler's own type.
function setResponsePane(pane: HttpResponsePane): void {
  patchHttpRequestTabState(props.tab.id, { responsePane: pane });
}

function viewHistory(): void {
  setResponsePane('history');
}

// D11: the two dead-end summaries become the way in to Timeline — the user who wonders about a
// number clicks *that number*, rather than hunting for a fifth segment among five.
function viewTimeline(): void {
  setResponsePane('timeline');
}

// Format detection and pretty text come from one worker pass per received body (P21 finding 7
// still holds: the pretty text is retained only while Pretty view is selected). `format` is
// `undefined` while pending.
const {
  format: prettyFormat,
  bodyText,
  pending: bodyPending,
} = useResponseBody(
  response,
  computed(() => props.tab.state.responseView),
);

const RESPONSE_VIEW_OPTIONS = computed(() => [
  {
    value: 'pretty' as const,
    label: t('studio.http.view.pretty.label'),
    testid: 'http-response-view-pretty',
  },
  { value: 'raw' as const, label: t('common.raw.label'), testid: 'http-response-view-raw' },
]);

function setResponseView(view: 'pretty' | 'raw'): void {
  patchHttpRequestTabState(props.tab.id, { responseView: view });
}

// The standing caption under the status row was reported as noise. The hint is now the status
// chip's tooltip, which is what the four other consumers of `statusHint` in this app
// (ResponseHistoryList, ResponseDiffDialog, TimelinePane) have always done with it.
const hint = computed(() => (response.value ? statusHint(response.value.status) : ''));

const redirectCaption = computed(() => {
  const r = response.value;
  if (!r || r.redirects.length === 0) return '';
  const n = r.redirects.length;
  return t('studio.http.response.redirects.status', { count: n, url: r.finalUrl });
});

// D12/D13: a view toggle, never an edit — Pretty shows the formatted body, Raw the bytes exactly
// as received. Neither ever mutates response.body itself (it is read-only runtime state, D6).

// P8 D10: the two storage notices — only meaningful while viewing a stored entry (the live
// response carries neither flag). Separate from, and additional to, bodyTruncated's own transfer
// message (F9) — one is about the transfer, the other about what history chose to keep.
const bodyStorageTruncated = computed(() => viewing.value?.snapshot.bodyStorageTruncated ?? false);
const requestFieldsElided = computed(() => viewing.value?.snapshot.requestFieldsElided ?? false);
const bodyNotStored = computed(() => (viewing.value ? !viewing.value.snapshot.bodyStored : false));

const viewingTime = computed(() => {
  const iso = viewing.value?.snapshot.entry.sentAt;
  if (!iso) return '';
  return formatClockTime(iso);
});

function onBackToLatest(): void {
  httpHistoryStore.backToLatest(props.tab.id);
}

// The response headers pane's own filter — matches name OR value (unlike the variables filter,
// a header's value is the thing being hunted, is server-provided, and it can never be a secret's
// plaintext). Per-tab runtime (P375), not tab state: a lens over what's on screen, not a setting —
// "a closed toolbar must never leave rows hidden with no visible cause" is satisfied trivially
// since the box is always visible while the pane is.
const headerFilter = computed({
  get: () => rt.value?.headerFilter ?? '',
  set: (v: string) => httpRequestViewStore.setHeaderFilter(props.tab.id, v),
});
const filteredHeaders = computed(() => {
  const q = headerFilter.value;
  return (response.value?.headers ?? []).filter(
    (h) => matchesQuery(h.name, q) || matchesQuery(h.value, q),
  );
});

// P16 D11: the find bar over the response body and the raw exchange's two documents. Component-
// local (not tab state, D11's own note): a lens over what's already on screen, not a setting.
const findOpen = ref(false);
function toggleFind(): void {
  findOpen.value = !findOpen.value;
}
function closeFind(): void {
  findOpen.value = false;
}

const bodyHostRef = ref<FindBarHost | null>(null);
const rawPaneRef = ref<{
  requestHost: FindBarHost | null;
  responseHost: FindBarHost | null;
  getDocs: () => { request: string; response: string };
} | null>(null);
// P28 D11: the options object joins the exposed pair — the painted highlight has to use the same three
// toggles the bar counts and steps through, or the two disagree about what a match even is.
const findBarRef = ref<{
  query: string;
  currentGlobal: number;
  options: FindOptions;
} | null>(null);

// D11: one target for the Body pane, two (request wire, response wire) for the Raw pane — the
// only two panes with a `rangeHighlights` compartment free (F12: the request body's own is taken
// by P15b's {{variable}} colouring).
const findTargets = computed<readonly FindBarTarget[]>(() => {
  if (!findOpen.value) return [];
  if (props.tab.state.responsePane === 'body') {
    return [{ doc: bodyText.value, host: bodyHostRef.value }];
  }
  if (props.tab.state.responsePane === 'raw') {
    const docs = rawPaneRef.value?.getDocs();
    if (!docs) return [];
    return [
      { doc: docs.request, host: rawPaneRef.value?.requestHost ?? null },
      { doc: docs.response, host: rawPaneRef.value?.responseHost ?? null },
    ];
  }
  return [];
});

// Paints exactly the matches the bar itself counts and steps through, onto whichever editor(s)
// are on screen. Reads `findBarRef`'s exposed `query`/`currentGlobal` (and `findTargets`)
// synchronously in this computed's own evaluation — not inside the closures it returns — so this
// recomputes, and so each editor's `rangeHighlights` prop reference changes (MonacoHost's own
// watch on that prop is what triggers a repaint), exactly when the query or the current match does.
const perTargetHighlighters = computed<((doc: string) => readonly RangeHighlight[])[]>(() => {
  const bar = findBarRef.value;
  const query = bar?.query ?? '';
  const currentGlobal = bar?.currentGlobal ?? -1;
  const options = bar?.options ?? DEFAULT_FIND_OPTIONS;
  const targets = findTargets.value;
  if (!query) return targets.map(() => () => []);
  let cursor = 0;
  return targets.map((t) => {
    const localCurrent = currentGlobal - cursor;
    cursor += findRanges(t.doc, query, undefined, options).length;
    return (doc: string): readonly RangeHighlight[] =>
      findRanges(doc, query, localCurrent, options);
  });
});
const bodyHighlights = computed(() => perTargetHighlighters.value[0]);
const rawRequestHighlights = computed(() => perTargetHighlighters.value[0]);
const rawResponseHighlights = computed(() => perTargetHighlighters.value[1]);

let unregisterCommands: Array<() => void> = [];
onMounted(() => {
  // D11: opened from the response status row's own search button, and by view.find — mirrors
  // DataView.vue's own registration, mounted only while this tab is the active one.
  unregisterCommands = [registerCommand('view.find', toggleFind)];
});
onUnmounted(() => {
  for (const off of unregisterCommands) off();
});
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="http-response-pane">
    <Alert
      strip
      v-if="rt?.status === 'error' && rt.error"
      variant="error"
      data-testid="http-send-error"
    >
      <AlertDescription>{{ rt.error.message }}</AlertDescription>
    </Alert>

    <ViewToolbar class="response-status-row">
      <template v-if="response">
        <Tooltip>
          <TooltipTrigger as-child>
            <Badge data :variant="statusClass(response.status)" data-testid="http-status">
              {{ response.status }} {{ response.statusText }}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{{ hint }}</TooltipContent>
        </Tooltip>
        <span class="ml-auto" />
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              variant="toolbar"
              size="status"
              data-testid="http-elapsed"
              @click="viewTimeline"
            >
              {{ formatDuration(response.elapsedMs) }}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('studio.http.response.timeTooltip.tooltip') }}</TooltipContent>
        </Tooltip>
        <span class="text-kira-sm text-subtle" data-testid="http-body-bytes">{{
          formatBytes(response.bodyBytes)
        }}</span>
        <SecondaryTabs
          :model-value="tab.state.responseView"
          :items="RESPONSE_VIEW_OPTIONS"
          data-testid="http-response-view-toggle"
          @update:model-value="(v) => setResponseView(v as 'pretty' | 'raw')"
        />
      </template>
      <span v-else class="ml-auto" />
      <!-- D11: only the two panes with a rangeHighlights compartment free (Body, Raw) get the
           find affordance — Headers has its own separate filter (D12), and History/Timeline are
           lists, not one searchable document. -->
      <TooltipIconButton
        v-if="tab.state.responsePane === 'body' || tab.state.responsePane === 'raw'"
        :icon="ICON.search"
        :label="t('studio.http.response.find.label')"
        :pressed="findOpen"
        data-testid="http-find-toggle"
        @click="toggleFind"
      />
      <SecondaryTabs
        :model-value="tab.state.responsePane"
        :items="RESPONSE_PANE_OPTIONS_WITH_COOKIES"
        data-testid="http-response-pane-toggle"
        @update:model-value="(v) => setResponsePane(v as HttpResponsePane)"
      />
    </ViewToolbar>

    <!-- The response search bar sat below/at the bottom of the response content instead of
         above it: the app's own established placement for a
         "search this content" bar — the SQL data view's SearchToolbar (DataView.vue's own
         `#strips` slot, docked "below the filter row, not floating over the grid it searches" —
         that same file's own comment records "the 'docks at the bottom of the result' placement
         ... overlapped the last visible row, which read as a bug rather than a search bar" and
         was rejected for exactly that reason), and the Mongo/documents view's SearchToolbar
         ("Below the filter/sort row, above the list it searches") — is directly below the toolbar
         it belongs to and above the content it searches, never below the content. This file's own
         previous placement (after the response body, right before ResponseDiffDialog) and
         ResponseFindBar.vue's own comment both cited "LAW 03: docks below the pane it searches" —
         backwards from what LAW 03 establishes everywhere else in this app. -->
    <ResponseFindBar
      v-if="findOpen && (tab.state.responsePane === 'body' || tab.state.responsePane === 'raw')"
      ref="findBarRef"
      :targets="findTargets"
      @close="closeFind"
    />

    <Alert strip v-if="viewing" variant="info" data-testid="http-history-band">
      <AlertDescription>
        {{
          t('studio.http.response.viewing.status', {
            time: viewingTime,
            method: viewing?.snapshot.entry.method ?? '',
            url: viewing?.snapshot.entry.url ?? '',
          })
        }}
      </AlertDescription>
      <AlertAction>
        <Button
          variant="toolbar"
          size="kira"
          data-testid="http-history-back"
          @click="onBackToLatest"
        >
          {{
            rt?.response
              ? t('studio.grpc.response.backToLatest.label')
              : t('common.action.close.label')
          }}
        </Button>
      </AlertAction>
    </Alert>

    <Alert strip v-if="response?.bodyTruncated" variant="warn" data-testid="http-body-truncated">
      <AlertDescription>
        {{
          t('studio.http.response.truncated.description', { size: formatBytes(response.bodyBytes) })
        }}
      </AlertDescription>
    </Alert>
    <Alert strip v-if="bodyStorageTruncated" variant="info" data-testid="http-history-truncated">
      <AlertDescription>
        {{ t('studio.http.response.bodyKept.description', { size: formatBytes(256 * 1024) }) }}
      </AlertDescription>
    </Alert>
    <Alert strip v-if="requestFieldsElided" variant="info" data-testid="http-history-fields-elided">
      <AlertDescription>
        {{ t('studio.http.history.fieldsElided.description') }}
      </AlertDescription>
    </Alert>
    <Alert strip v-if="bodyNotStored" variant="info" data-testid="http-history-binary-note">
      <AlertDescription>
        {{
          t('studio.http.response.binaryNotKept.description', {
            size: response ? formatBytes(response.bodyBytes) : '',
          })
        }}
      </AlertDescription>
    </Alert>
    <Tooltip v-if="redirectCaption">
      <TooltipTrigger as-child>
        <Button
          variant="toolbar"
          size="status"
          class="w-full justify-start"
          data-testid="http-redirects"
          @click="viewTimeline"
        >
          {{ redirectCaption }}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{{ t('studio.http.response.timeTooltip.tooltip') }}</TooltipContent>
    </Tooltip>

    <ResponseHistoryList
      v-if="tab.state.responsePane === 'history'"
      :tab="tab"
      @compare="onCompare"
    />
    <div
      v-else-if="tab.state.responsePane === 'headers'"
      class="flex flex-1 min-h-0 flex-col"
      data-testid="http-response-headers"
    >
      <template v-if="response">
        <SearchField
          v-model="headerFilter"
          :placeholder="t('studio.http.response.headersFilter.placeholder')"
          data-testid="http-response-headers-filter"
        />
        <span
          v-if="headerFilter.trim()"
          class="text-kira-sm text-subtle px-1.5 pt-1 pb-0"
          data-testid="http-response-headers-count"
        >
          {{
            t('studio.http.response.headersCount.status', {
              shown: filteredHeaders.length,
              count: response.headers.length,
            })
          }}
        </span>
        <div class="flex flex-1 min-h-0 flex-col gap-0.5 overflow-auto p-1.5">
          <div
            v-for="(h, i) in filteredHeaders"
            :key="i"
            class="flex gap-1.5 text-kira-sm"
            data-testid="response-header-row"
          >
            <span class="text-muted-foreground shrink-0 min-w-40 font-data">{{ h.name }}</span>
            <span class="wrap-anywhere font-data">{{ h.value }}</span>
          </div>
        </div>
      </template>
      <EmptyState v-else icon="arrow-right" :title="t('studio.common.responseEmpty.empty')" />
    </div>
    <RawExchangePane
      v-else-if="tab.state.responsePane === 'raw'"
      ref="rawPaneRef"
      :tab="tab"
      :request-highlights="rawRequestHighlights"
      :response-highlights="rawResponseHighlights"
    />
    <TimelinePane v-else-if="tab.state.responsePane === 'timeline'" :tab="tab" />
    <CookiesPane
      v-else-if="tab.state.responsePane === 'cookies'"
      mode="response"
      :tab-id="tab.id"
      :response="response"
    />
    <div v-else class="response-body flex flex-1 min-h-0 flex-col">
      <template v-if="response">
        <span
          v-if="response.bodyEncoding === 'base64'"
          class="text-kira-sm text-muted-foreground p-1.5"
          data-testid="http-response-binary"
        >
          {{ t('studio.http.response.binary.status', { count: response.bodyBytes }) }}
        </span>
        <InlineState v-else-if="bodyPending" kind="loading" data-testid="http-response-formatting">
          {{ t('studio.http.response.formatting.loading') }}
        </InlineState>
        <MonacoHost
          v-else
          ref="bodyHostRef"
          :doc="bodyText"
          :language="prettyFormat ?? 'plain'"
          readonly
          :range-highlights="bodyHighlights"
        />
      </template>
      <EmptyState v-else icon="arrow-right" :title="t('studio.common.responseEmpty.empty')">
        <Button
          v-if="hasHistory"
          variant="link"
          size="inline"
          class="mt-1"
          data-testid="http-history-hint"
          @click="viewHistory"
        >
          {{ t('studio.http.response.viewHistory.label', { count: historyCount }) }}
        </Button>
      </EmptyState>
    </div>

    <ResponseDiffDialog v-if="compareIds" :ids="compareIds" @close="closeCompare" />
    <!-- `.response-status-row`/`.response-body` stay bare markers (test locators).
         `.pane-jump-link:hover` moved onto each button's own `hover:text-fg` (a plain button reset
         rather than AppButton's own chrome, so the status row's look is unchanged) -- no test
         locates by that class, so the marker drops. -->
  </div>
</template>
