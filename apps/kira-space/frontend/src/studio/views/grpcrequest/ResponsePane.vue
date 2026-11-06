<script setup lang="ts">
import {
  GRPC_CODE_NAMES,
  type GrpcResponsePane,
  grpcCodeClass,
  grpcCodeHint,
} from '@shared/domain/grpc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatBytes, formatClockTime, formatDuration } from '@workbench/behaviour/format';
import { useVirtualRows } from '@workbench/behaviour/virtualRows';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import type { RangeHighlight } from '@workbench/editor/ranges';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import type { GrpcRequestTabRecord } from '../../../state/tabDomain';
import { patchGrpcRequestTabState } from '../../api/tabs';
import { DEFAULT_FIND_OPTIONS, type FindOptions, findRanges } from '../../editor/findRanges';
import MonacoHost from '../../editor/MonacoHost.vue';
import ResponseFindBar, {
  type FindBarHost,
  type FindBarTarget,
} from '../shared/ResponseFindBar.vue';
import CallHistoryList from './CallHistoryList.vue';
import { useGrpcCallHistoryStore, useGrpcHistoryList, useGrpcHistoryViewing } from './history';
import { useGrpcRequestViewStore } from './state';

// D14: three segments — Messages · Metadata · History — and deliberately no Raw, no Timeline (D14
// answers P9 OQ-9/P10 OQ-8: absent, not a degraded pane — the message list and this status line
// already are the honest view for this protocol, F7).
const props = defineProps<{ tab: GrpcRequestTabRecord }>();
const grpcRequestViewStore = useGrpcRequestViewStore();
const grpcCallHistoryStore = useGrpcCallHistoryStore();

const rt = computed(() => grpcRequestViewStore.runtime[props.tab.id]);
// P175 D4: HTTP ResponsePane.vue's own history queries, restated for gRPC.
const { entries: historyEntries } = useGrpcHistoryList(() => props.tab);
const { viewing } = useGrpcHistoryViewing(() => props.tab.id);

// D10: the source swap — a selected history entry's snapshot, or the live call, or none.
const liveResult = computed(() => rt.value?.result ?? null);
const liveMessages = computed(() => rt.value?.messages ?? []);

const hasHistory = computed(() => historyEntries.value.length > 0);
const historyCount = computed(() => historyEntries.value.length);

const codeName = computed(() => {
  if (viewing.value) return viewing.value.snapshot.entry.codeName;
  if (liveResult.value)
    return liveResult.value.codeName || GRPC_CODE_NAMES[liveResult.value.code] || 'UNKNOWN';
  return '';
});
const code = computed(() =>
  viewing.value ? viewing.value.snapshot.entry.code : (liveResult.value?.code ?? 0),
);
const statusMessage = computed(() =>
  viewing.value
    ? viewing.value.snapshot.entry.statusMessage
    : (liveResult.value?.statusMessage ?? ''),
);
const elapsedMs = computed(() =>
  viewing.value ? viewing.value.snapshot.entry.elapsedMs : (liveResult.value?.elapsedMs ?? 0),
);
// Finding 11: rt.messageBytes is a running total kept incrementally by state.ts's own push
// handler — reading it here is O(1); the old `rt.value.messages.reduce(...)` re-summed the whole
// (potentially 10,000-message) array on every single push.
const messageBytes = computed(() =>
  viewing.value ? viewing.value.snapshot.entry.messageBytes : (rt.value?.messageBytes ?? 0),
);
// A stored message cut at the 64 KiB per-message cap carries its own `truncated` flag
// (grpc-history.ts's own GrpcCallHistoryMessage) — this used to drop it, rendering the cut prefix
// in Monaco as if it were the complete, valid JSON body. A live message is never truncated (the
// cap only applies to what History stores), so it maps in as `false`.
const messages = computed(() =>
  viewing.value
    ? viewing.value.snapshot.messages.map((m) => ({
        seq: m.seq,
        json: m.json,
        wireBytes: m.wireBytes,
        offsetMs: m.offsetMs,
        truncated: m.truncated,
      }))
    : liveMessages.value.map((m) => ({ ...m, truncated: false })),
);
// The stored request message's own truncation flag (requestMessageTruncated) had zero
// frontend readers — surfaced here as a note next to the "viewing a history call" band, since this
// pane does not render the stored request message body itself (the message list is the
// response's own messages; showing the request body too is out of scope for this fix).
const requestMessageTruncated = computed(
  () => viewing.value?.snapshot.requestMessageTruncated ?? false,
);
const metadataElided = computed(() => viewing.value?.snapshot.metadataElided ?? false);
// The summary line used to read messages.length — capped at MAX_LIVE_MESSAGES for a live call
// (state.ts) and at maxGrpcStoredMessages for a stored entry — right next to a separate
// elided-messages strip stating the true, uncapped total, two contradictory counts on screen at
// once for the same response. Sourced from the true total instead: rt.trueMessageCount for live,
// the stored entry's own true messageCount for history.
const messageCount = computed(() =>
  viewing.value ? viewing.value.snapshot.entry.messageCount : (rt.value?.trueMessageCount ?? 0),
);
// D15/D17: the live view keeps only the most recent MAX_LIVE_MESSAGES (state.ts) — this is true
// only for a live, still/just-streamed call, never a stored history entry's own capped snapshot
// (finding 8's own "first N of M" note above covers that case with its own wording).
const liveMessagesElided = computed(
  () => !viewing.value && (rt.value?.trueMessageCount ?? 0) > messages.value.length,
);
const header = computed(() =>
  viewing.value ? viewing.value.snapshot.header : (liveResult.value?.header ?? []),
);
const trailer = computed(() =>
  viewing.value ? viewing.value.snapshot.trailer : (liveResult.value?.trailer ?? []),
);

// P18 D14: the chip/hint/elapsed/summary group only makes sense once there is a code to show —
// same gate the template already used for that group, named here so the hint computed can share
// it without duplicating the condition.
const hasCode = computed(() => !!liveResult.value || !!viewing.value);
// P28 D1 reverses P18 D13's always-visible line, matching the identical change on the HTTP
// response pane: the meaning is now the status chip's tooltip. The server's own statusMessage
// keeps its own line — that is a message from the server, not a restatement of the code.
const codeHint = computed(() => (hasCode.value ? grpcCodeHint(code.value) : ''));

const RESPONSE_PANE_OPTIONS = computed(() => [
  {
    value: 'messages' as const,
    label: t('studio.grpc.response.pane.messages.option'),
    testid: 'grpc-response-pane-messages',
  },
  {
    value: 'metadata' as const,
    label: t('studio.grpc.pane.metadata.option'),
    testid: 'grpc-response-pane-metadata',
  },
  {
    value: 'history' as const,
    label: t('common.module.history.label'),
    testid: 'grpc-response-pane-history',
  },
]);

function setResponsePane(pane: GrpcResponsePane): void {
  patchGrpcRequestTabState(props.tab.id, { responsePane: pane });
}

function viewHistory(): void {
  setResponsePane('history');
}

const expanded = ref<Set<number>>(new Set());
function toggleExpanded(seq: number): void {
  const next = new Set(expanded.value);
  if (next.has(seq)) next.delete(seq);
  else next.add(seq);
  expanded.value = next;
}

// The message list is virtualized (below), which needs one known height per row —
// the density row height for a collapsed header, or that plus MESSAGE_DETAIL_HEIGHT for an expanded
// one (a fixed, internally-scrollable box, not an auto-growing one — the same "one resolved
// height per row" contract OperationsPanel.vue's own expandable row already follows).
const { single: messageRowHeight } = useRowHeight();
const MESSAGE_DETAIL_HEIGHT = 200;
const messageRowHeights = computed(() =>
  messages.value.map((m) =>
    expanded.value.has(m.seq)
      ? messageRowHeight.value + MESSAGE_DETAIL_HEIGHT
      : messageRowHeight.value,
  ),
);

// P104 §3.4: useVirtualRows replaces VirtualList.vue at this call site. rowHeights carries the
// actual per-row height (P27 D18's contract), so toggling one row's expanded state re-measures.
const scrollRef = ref<HTMLElement | null>(null);
const { virtualItems: virtualMessages, totalSize: totalMessagesSize } = useVirtualRows({
  count: () => messages.value.length,
  rowHeight: () => messageRowHeight.value,
  rowHeights: () => messageRowHeights.value,
  scrollElement: scrollRef,
  overscan: 8,
});
const visibleMessages = computed(() =>
  virtualMessages.value.map((row) => ({ row, m: messages.value[row.index] })),
);
// D14: a unary call is the same pane with exactly one entry, expanded.
watch(
  messages,
  (msgs) => {
    if (msgs.length === 1) expanded.value = new Set([msgs[0].seq]);
  },
  { immediate: true },
);

function onBackToLatest(): void {
  grpcCallHistoryStore.backToLatestGrpc(props.tab.id);
}

// Find-in-message — HTTP's own ResponsePane.vue find bar, applied to the one surface that is the
// real gap: the message list has no search at all. The message list is a VirtualList (only visible
// rows are ever mounted), so unlike HTTP's single always-mounted body this can only search
// whichever message is expanded and on screen right now — the lowest-seq expanded one,
// deterministic when more than one is open. A unary call's own auto-expand (above) makes this the
// common case land exactly right with no extra action.
const findOpen = ref(false);
function toggleFind(): void {
  findOpen.value = !findOpen.value;
}
function closeFind(): void {
  findOpen.value = false;
}

const targetSeq = computed<number | null>(() => {
  if (expanded.value.size === 0) return null;
  return Math.min(...expanded.value);
});
const targetMessage = computed(() => messages.value.find((m) => m.seq === targetSeq.value) ?? null);

// Function refs, not a single ref — VirtualList only mounts the rows currently on screen, so the
// host for `targetSeq` may not exist at all (scrolled out) even while its seq is in `expanded`.
// `scrollRangeIntoView` on a null host is a no-op via optional chaining below, same as
// HTTP's own bodyHostRef before its first render.
const messageHosts = new Map<number, FindBarHost>();
function setMessageHost(seq: number, el: unknown): void {
  if (el && typeof el === 'object' && 'scrollRangeIntoView' in el) {
    messageHosts.set(seq, el as FindBarHost);
  } else {
    messageHosts.delete(seq);
  }
}

// P28 D11: the options object joins the exposed pair — see the HTTP pane's own note.
const findBarRef = ref<{
  query: string;
  currentGlobal: number;
  options: FindOptions;
} | null>(null);
const findTargets = computed<readonly FindBarTarget[]>(() => {
  if (!findOpen.value || !targetMessage.value) return [];
  const seq = targetMessage.value.seq;
  return [{ doc: targetMessage.value.json, host: messageHosts.get(seq) ?? null }];
});
const messageHighlights = computed<(doc: string) => readonly RangeHighlight[]>(() => {
  const bar = findBarRef.value;
  const query = bar?.query ?? '';
  if (!query || findTargets.value.length === 0) return () => [];
  const currentGlobal = bar?.currentGlobal ?? -1;
  const options = bar?.options ?? DEFAULT_FIND_OPTIONS;
  return (doc: string) => findRanges(doc, query, currentGlobal, options);
});

const viewingTime = computed(() => {
  const iso = viewing.value?.snapshot.entry.calledAt;
  if (!iso) return '';
  return formatClockTime(iso);
});

let unregisterCommands: Array<() => void> = [];
onMounted(() => {
  // D14: mirrors HTTP's own ResponsePane.vue registration — opened from the status row's own
  // search button, and by view.find, mounted only while this tab is the active one.
  unregisterCommands = [registerCommand('view.find', toggleFind)];
});
onUnmounted(() => {
  for (const off of unregisterCommands) off();
});
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="grpc-response-pane">
    <Alert
      strip
      v-if="rt?.status === 'error' && rt.error"
      variant="error"
      data-testid="grpc-call-error"
    >
      <AlertDescription>{{ rt.error.message }}</AlertDescription>
    </Alert>

    <!-- P18 D14 (P15 D1's gRPC sibling): the status row, the pane switcher and every strip render
         from tab-open — only the response-dependent *contents* below stay conditional. A freshly-
         opened tab used to show no Messages/Metadata/History switcher at all. -->
    <!-- `.response-status-row` stays a bare marker: api-ui-consistency.spec.ts locates it directly. -->
    <ViewToolbar class="response-status-row">
      <template v-if="hasCode">
        <Tooltip v-if="codeHint">
          <TooltipTrigger as-child>
            <Badge data :variant="grpcCodeClass(code)" data-testid="grpc-status-chip">
              {{ codeName }}
              ({{ code }})
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{{ codeHint }}</TooltipContent>
        </Tooltip>
        <Badge data v-else :variant="grpcCodeClass(code)" data-testid="grpc-status-chip">
          {{ codeName }}
          ({{ code }})
        </Badge>
        <span class="ml-auto" />
        <span class="text-kira-sm text-subtle" data-testid="grpc-elapsed">{{
          formatDuration(elapsedMs)
        }}</span>
        <span class="text-kira-sm text-subtle" data-testid="grpc-message-summary">
          {{
            t('studio.grpc.response.summary.status', {
              count: messageCount,
              size: formatBytes(messageBytes),
            })
          }}
        </span>
      </template>
      <span v-else class="ml-auto" />
      <!-- P22b D14: HTTP's own ResponsePane.vue idiom — only the Messages pane has a document to
           search (Metadata is a plain key-value list, History is a row list). -->
      <TooltipIconButton
        v-if="tab.state.responsePane === 'messages'"
        :icon="ICON.search"
        :label="t('studio.grpc.response.find.label')"
        :pressed="findOpen"
        data-testid="grpc-find-toggle"
        @click="toggleFind"
      />
      <SecondaryTabs
        :model-value="tab.state.responsePane"
        :items="RESPONSE_PANE_OPTIONS"
        data-testid="grpc-response-pane-toggle"
        @update:model-value="(v) => setResponsePane(v as GrpcResponsePane)"
      />
    </ViewToolbar>

    <!-- P18 D13's other half, kept: the server's own statusMessage is a message, not a
         restatement of the code, so it stays on its own line free to wrap. -->
    <div
      v-if="statusMessage"
      class="text-kira-sm text-subtle px-1.5 pt-0 pb-1"
      data-testid="grpc-status-message"
      >{{
        statusMessage
      }}</div
    >

    <Alert strip v-if="viewing" variant="info" data-testid="grpc-history-band">
      <AlertDescription>{{
        t('studio.grpc.response.viewing.status', {
          time: viewingTime,
          method: viewing?.snapshot.method ?? '',
        })
      }}</AlertDescription>
      <AlertAction>
        <Button
          variant="toolbar"
          size="kira"
          data-testid="grpc-history-back"
          @click="onBackToLatest"
        >
          {{
            rt?.result
              ? t('studio.grpc.response.backToLatest.label')
              : t('common.action.close.label')
          }}
        </Button>
      </AlertAction>
    </Alert>

    <!-- D11: a streaming call's history entry stores only the first maxGrpcStoredMessages
         (finding 8) — this is the one place that ever becomes visible now that ServerStream
         actually fills Messages/MessageCount in. -->
    <Alert
      strip
      v-if="viewing?.snapshot.messagesElided"
      variant="info"
      data-testid="grpc-history-messages-elided"
    >
      <AlertDescription>
        {{
          t('studio.grpc.response.firstMessages.description', {
            shown: messages.length,
            total: viewing.snapshot.entry.messageCount,
          })
        }}
      </AlertDescription>
    </Alert>

    <!-- P108 F18: the stored request message was cut at the 256 KiB per-entry cap; this pane does
         not render the request body itself (see the computed above), so the note is the only
         surface for requestMessageTruncated. -->
    <Alert
      strip
      v-if="requestMessageTruncated"
      variant="info"
      data-testid="grpc-request-message-truncated"
    >
      <AlertDescription>
        {{ t('studio.grpc.response.requestCut.description', { size: formatBytes(256 * 1024) }) }}
      </AlertDescription>
    </Alert>

    <Alert strip v-if="metadataElided" variant="info" data-testid="grpc-history-metadata-elided">
      <AlertDescription>
        {{ t('studio.grpc.response.metadataElided.description') }}
      </AlertDescription>
    </Alert>

    <Alert strip v-if="rt?.status === 'cancelled'" variant="warn" data-testid="grpc-stopped-strip">
      <AlertDescription>
        {{ t('studio.grpc.response.stopped.description', { count: messages.length }) }}
      </AlertDescription>
    </Alert>

    <!-- D15/D17: the live view's own ceiling (state.ts's MAX_LIVE_MESSAGES) — finding 11. -->
    <Alert strip v-if="liveMessagesElided" variant="info" data-testid="grpc-live-messages-elided">
      <AlertDescription>
        {{
          t('studio.grpc.response.recentMessages.description', {
            shown: messages.length,
            total: rt?.trueMessageCount ?? 0,
          })
        }}
      </AlertDescription>
    </Alert>

    <CallHistoryList v-if="tab.state.responsePane === 'history'" :tab="tab" />
    <div
      v-else-if="tab.state.responsePane === 'metadata'"
      class="flex flex-1 min-h-0 flex-col gap-2 overflow-auto p-1.5"
      data-testid="grpc-response-metadata"
    >
      <div class="metadata-group">
        <div class="mb-0.5 text-kira-sm text-muted-foreground">{{
          t('studio.grpc.response.header.label')
        }}</div>
        <div v-for="(h, i) in header" :key="`h${i}`" class="flex gap-1.5 text-kira-sm">
          <span class="text-muted-foreground shrink-0 min-w-40 font-data">{{ h.name }}</span>
          <span class="wrap-anywhere font-data">{{ h.value }}</span>
        </div>
        <div v-if="header.length === 0" class="text-kira-sm text-subtle">{{
          t('studio.grpc.response.noHeader.empty')
        }}</div>
      </div>
      <div class="metadata-group">
        <div class="mb-0.5 text-kira-sm text-muted-foreground">{{
          t('studio.grpc.response.trailer.label')
        }}</div>
        <div v-for="(tr, i) in trailer" :key="`t${i}`" class="flex gap-1.5 text-kira-sm">
          <span class="text-muted-foreground shrink-0 min-w-40 font-data">{{ tr.name }}</span>
          <span class="wrap-anywhere font-data">{{ tr.value }}</span>
        </div>
        <div v-if="trailer.length === 0" class="text-kira-sm text-subtle">{{
          t('studio.grpc.response.noTrailer.empty')
        }}</div>
      </div>
    </div>
    <div v-else class="flex flex-1 min-h-0 flex-col" data-testid="grpc-message-list">
      <!-- P104 §3.4: the scroll element @tanstack/vue-virtual measures and virtualizes against —
           this component owns it directly, VirtualList.vue no longer wraps it. -->
      <div
        v-if="messages.length > 0"
        ref="scrollRef"
        class="flex-1 min-h-0 overflow-auto"
        data-testid="virtual-list"
      >
        <div class="relative w-full" :style="{ height: `${totalMessagesSize}px` }">
          <div
            v-for="entry in visibleMessages"
            :key="entry.m.seq"
            class="flex flex-col absolute top-0 left-0 w-full"
            data-testid="grpc-message-entry"
            :style="{ transform: `translateY(${entry.row.start}px)` }"
          >
            <!-- height (not padding) so this row's own rendered height stays exactly the density
                 row height — the virtualizer positions every row assuming it, border included via
                 box-sizing. -->
            <button
              type="button"
              :class="cn(rowVariants(), 'box-border w-full rounded-none bg-none px-1.5')"
              :style="{ height: `${messageRowHeight}px` }"
              @click="toggleExpanded(entry.m.seq)"
            >
              <span class="text-kira-sm text-subtle" data-testid="grpc-message-offset">{{
                t('studio.grpc.response.offset.status', {
                  duration: formatDuration(entry.m.offsetMs),
                })
              }}</span>
              <span class="text-kira-sm text-subtle">{{ formatBytes(entry.m.wireBytes) }}</span>
              <Tooltip v-if="entry.m.truncated">
                <TooltipTrigger as-child>
                  <CodiconIcon
                    :name="ICON.warning"
                    :size="12"
                    data-testid="grpc-message-truncated"
                  />
                </TooltipTrigger>
                <TooltipContent>{{
                  t('studio.grpc.response.messageCut.tooltip', { size: formatBytes(64 * 1024) })
                }}</TooltipContent>
              </Tooltip>
              <span class="ml-auto" />
              <span class="text-kira-sm text-subtle">#{{ entry.m.seq }}</span>
            </button>
            <!-- Fixed height (not auto-grow) for the same reason .message-header's is — MUST stay
                 numerically equal to the script's own MESSAGE_DETAIL_HEIGHT (200px); a JSON
                 document taller than this scrolls inside MonacoHost's own scroller instead of
                 growing the row. -->
            <div v-if="expanded.has(entry.m.seq)" class="box-border h-50 overflow-auto">
              <MonacoHost
                :ref="(el) => setMessageHost(entry.m.seq, el)"
                :doc="entry.m.json"
                language="json"
                readonly
                :range-highlights="entry.m.seq === targetSeq ? messageHighlights : undefined"
              />
            </div>
          </div>
        </div>
      </div>
      <EmptyState
        v-if="messages.length === 0"
        icon="arrow-right"
        :title="t('studio.grpc.response.empty')"
      >
        <Button
          v-if="hasHistory"
          variant="link"
          size="inline"
          class="mt-1"
          data-testid="grpc-history-hint"
          @click="viewHistory"
        >
          {{ t('studio.grpc.response.viewHistory.label', { count: historyCount }) }}
        </Button>
      </EmptyState>
    </div>

    <!-- D14: docked below the pane it searches (LAW 03), mirroring HTTP's own ResponsePane.vue. -->
    <ResponseFindBar
      v-if="findOpen && tab.state.responsePane === 'messages'"
      ref="findBarRef"
      :targets="findTargets"
      @close="closeFind"
    />
    <!-- P110 I2-17: `.message-header:hover` moved onto the button's own the row recipe -- no
         test locates by that class, so the marker drops. -->
  </div>
</template>
