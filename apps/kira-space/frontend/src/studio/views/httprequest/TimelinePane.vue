<script setup lang="ts">
import type { HttpTimelineHop } from '@shared/domain/http';
import { statusClass, statusHint } from '@shared/domain/http';
import Disclosure from '@theme/components/Disclosure.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Card } from '@theme/components/ui/card';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { formatDuration } from '@workbench/behaviour/format';
import { computed } from 'vue';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import { useHttpHistoryViewing } from './history';
import { useHttpRequestViewStore } from './state';

// P10 D11/D12/D13/F18: the waterfall and the per-hop detail — a fifth response-pane segment,
// mounted here (not http/) for the same reason RawExchangePane.vue is (P9 F16): it needs
// theme/ and views/**'s own import rights, which http/** does not have (biome.json).
const props = defineProps<{ tab: HttpRequestTabRecord }>();
const httpRequestViewStore = useHttpRequestViewStore();

// P8 D10's own source swap, duplicated exactly as RawExchangePane.vue does — each pane computes
// its own runtime over the tab id rather than threading it down as props.
const rt = computed(() => httpRequestViewStore.runtime[props.tab.id]);
const { viewing: viewingStored } = useHttpHistoryViewing(() => props.tab.id);
const response = computed(
  () => viewingStored.value?.snapshot.response ?? rt.value?.response ?? null,
);
// D10: unlike wire (P9 D7), a stored entry keeps its real timeline — there is no "no timeline for
// a stored response" empty state distinct from "no timeline at all" the way Raw has one.
const timeline = computed(() => response.value?.timeline ?? null);

// P10 D15/C5: a failed send's own partial timeline — closes P9 OQ-7. A response, live or stored,
// always takes precedence: this is only ever consulted when there is none.
const failedTimeline = computed(() => {
  if (response.value) return null;
  if (rt.value?.status !== 'error') return null;
  return rt.value.error?.timeline ?? null;
});
const activeTimeline = computed(() => timeline.value ?? failedTimeline.value);

// D13: the failure sentence — inferred from which of the send's own measured
// phases the failed hop actually has, the same evidence the hop's own phase list already shows.
function failureText(hop: HttpTimelineHop): string {
  if (!hop.dns && !hop.connect) return t('studio.http.timeline.failedConnect.description');
  if (hop.connect && !hop.tls && hop.url.startsWith('https:'))
    return t('studio.http.timeline.failedTls.description');
  if (!hop.wait && !hop.download) return t('studio.http.timeline.failedWait.description');
  return t('studio.http.timeline.failedDownload.description');
}

const summary = computed(() => {
  const tl = timeline.value;
  if (!tl) return '';
  const n = tl.hops.length;
  const key = tl.hops[0]?.reused
    ? 'studio.http.timeline.summaryReused.status'
    : 'studio.http.timeline.summary.status';
  return t(key, { count: n, total: formatDuration(tl.totalMs) });
});

interface PhaseSegment {
  key: 'dns' | 'connect' | 'tls' | 'wait' | 'download';
  labelKey: MessageKey;
  bgClass: string;
}
const PHASE_SEGMENTS: readonly PhaseSegment[] = [
  { key: 'dns', labelKey: 'studio.http.timeline.phase.dns.label', bgClass: 'bg-conn-violet' },
  { key: 'connect', labelKey: 'common.action.connect.label', bgClass: 'bg-conn-blue' },
  { key: 'tls', labelKey: 'studio.common.tls.label', bgClass: 'bg-conn-teal' },
  { key: 'wait', labelKey: 'studio.http.timeline.phase.wait.label', bgClass: 'bg-conn-amber' },
  { key: 'download', labelKey: 'common.action.download.label', bgClass: 'bg-conn-green' },
];
// D12: a hop whose own total is a rounding sliver of the send still renders a visible bar, so a
// sub-millisecond reused hop is seen rather than a hairline.
const MIN_BAR_PCT = 2;

function pct(part: number, whole: number): number {
  if (whole <= 0) return 0;
  return (part / whole) * 100;
}

/** D5: the hop's own bar, offset by its StartOffsetMs inside the send's full-width track. */
function hopBarStyle(hop: HttpTimelineHop): { left: string; width: string } {
  const total = activeTimeline.value?.totalMs ?? 0;
  return {
    left: `${pct(hop.startOffsetMs, total)}%`,
    width: `${Math.max(pct(hop.totalMs, total), MIN_BAR_PCT)}%`,
  };
}

/** D5: the five phases plus a trailing, unlabelled "residue" segment for whatever time inside the
 *  hop's own total is not attributed to any of them — the CONNECT-tunnel gap F12 measured, mostly.
 *  Never padded to make the bar reach the end; the residue segment *is* the honest admission. */
function hopSegments(hop: HttpTimelineHop): Array<{ bgClass: string; widthPct: number }> {
  const total = hop.totalMs;
  const segs = PHASE_SEGMENTS.map((s) => ({
    bgClass: s.bgClass,
    widthPct: pct(hop[s.key]?.durationMs ?? 0, total),
  }));
  const measured = segs.reduce((sum, s) => sum + s.widthPct, 0);
  const residuePct = Math.max(0, 100 - measured);
  return [...segs, { bgClass: 'bg-conn-grey', widthPct: residuePct }];
}

function hopHeaders(hop: HttpTimelineHop): Array<{ name: string; value: string }> {
  if (hop.headers && hop.headers.length > 0) return hop.headers;
  const last = hop.index === (activeTimeline.value?.hops.length ?? 0) - 1;
  return last ? (response.value?.headers ?? []) : [];
}

function residueMs(hop: HttpTimelineHop): number {
  const measured =
    (hop.dns?.durationMs ?? 0) +
    (hop.connect?.durationMs ?? 0) +
    (hop.tls?.durationMs ?? 0) +
    (hop.wait?.durationMs ?? 0) +
    (hop.download?.durationMs ?? 0);
  return Math.max(0, hop.totalMs - measured);
}
function residueIsNotable(hop: HttpTimelineHop): boolean {
  return hop.totalMs > 0 && residueMs(hop) / hop.totalMs > 0.05;
}

// D13's own table, collected here so the prose is written once and can be checked against the
// measurements it cites.
function phaseTooltip(hop: HttpTimelineHop, key: PhaseSegment['key']): string {
  if (key === 'dns') {
    return t(
      hop.reused
        ? 'studio.http.timeline.noDnsReused.tooltip'
        : 'studio.http.timeline.noDnsIp.tooltip',
    );
  }
  if (key === 'connect') return t('studio.http.timeline.noConnect.tooltip');
  if (key === 'tls') {
    return t(
      hop.reused
        ? 'studio.http.timeline.noTlsReused.tooltip'
        : 'studio.http.timeline.noTlsPlain.tooltip',
    );
  }
  if (key === 'download') {
    return t('studio.http.timeline.noDownload.tooltip');
  }
  // wait
  return t('studio.http.timeline.noWait.tooltip');
}

function reuseNote(hop: HttpTimelineHop): string {
  if (!hop.reused) {
    if (!hop.dns && !hop.connect) return t('studio.http.timeline.noDnsIp.tooltip');
    return '';
  }
  return hop.idleMs !== undefined
    ? t('studio.http.timeline.reusedIdle.hint', { idle: formatDuration(hop.idleMs) })
    : t('studio.http.timeline.reused.hint');
}

function attemptsNote(hop: HttpTimelineHop): string {
  if (hop.connAttempts <= 1) return '';
  return t('studio.http.timeline.attempts.hint', { count: hop.connAttempts });
}

function info1xxNote(hop: HttpTimelineHop): string {
  if (!hop.info1xx || hop.info1xx.length === 0) return '';
  return t('studio.http.timeline.info1xx.hint', { codes: hop.info1xx.join(', ') });
}

function residueNote(hop: HttpTimelineHop): string {
  if (!residueIsNotable(hop)) return '';
  return t('studio.http.timeline.residue.hint', { duration: formatDuration(residueMs(hop)) });
}

// A "note" that fires on the common case (a reused connection is the common case) is a
// caption, not a full-width tinted banner — up to five MessageStrips per hop could push ~180px
// of banner between a 10px bar and the next hop's caption. Same four sentences, same order, same
// testids — now text-kira-sm dim caption lines instead of Alert-strip banners.
interface HopNote {
  testid: string;
  text: string;
}
function hopNotes(hop: HttpTimelineHop): HopNote[] {
  const notes: HopNote[] = [];
  const reuse = reuseNote(hop);
  if (reuse) notes.push({ testid: 'http-timeline-reuse-note', text: reuse });
  const attempts = attemptsNote(hop);
  if (attempts) notes.push({ testid: 'http-timeline-attempts-note', text: attempts });
  const info1xx = info1xxNote(hop);
  if (info1xx) notes.push({ testid: 'http-timeline-1xx-note', text: info1xx });
  const residue = residueNote(hop);
  if (residue) notes.push({ testid: 'http-timeline-gap-note', text: residue });
  if (hop.headersElided) {
    notes.push({
      testid: 'http-timeline-headers-elided-note',
      text: t('studio.http.timeline.headersElided.hint'),
    });
  }
  return notes;
}
</script>

<template>
  <div
    class="flex flex-1 min-h-0 flex-col gap-1 overflow-auto p-1.5"
    data-testid="http-timeline-pane"
  >
    <template v-if="activeTimeline && activeTimeline.hops.length > 0">
      <!-- D15: a failed send's own partial timeline — the failure sentence names the phase the
           request never got past, from the same measured phases the hop below already shows. -->
      <Alert v-if="failedTimeline" variant="error" data-testid="http-timeline-failure-note">
        <AlertDescription>
          {{ failureText(failedTimeline.hops[failedTimeline.hops.length - 1]) }}
        </AlertDescription>
      </Alert>
      <Alert v-else-if="viewingStored" variant="info" data-testid="http-timeline-stored-note">
        <AlertDescription>
          {{ t('studio.http.timeline.recorded.description') }}
        </AlertDescription>
      </Alert>

      <div v-if="!failedTimeline" class="flex items-center justify-between gap-1.5">
        <div class="text-kira-sm text-subtle px-0.5" data-testid="http-timeline-summary">
          {{ summary }}
        </div>
        <div
          class="flex flex-wrap gap-1.5 px-0.5 text-kira-sm text-subtle"
          data-testid="http-timeline-legend"
        >
          <span
            v-for="seg in PHASE_SEGMENTS"
            :key="seg.key"
            class="inline-flex items-center gap-0.5"
          >
            <span class="inline-block h-2 w-2 rounded-kira-sm" :class="seg.bgClass" />
            {{ t(seg.labelKey) }}
          </span>
          <span class="inline-flex items-center gap-0.5">
            <span class="inline-block h-2 w-2 rounded-kira-sm bg-conn-grey" />
            {{ t('studio.http.timeline.unattributed.label') }}
          </span>
        </div>
      </div>

      <div class="flex flex-col gap-1.5">
        <Card
          v-for="hop in activeTimeline.hops"
          :key="hop.index"
          class="gap-0.5"
          data-testid="http-timeline-hop"
        >
          <div
            class="flex items-center gap-1 text-kira-sm font-data"
            data-testid="http-timeline-hop-caption"
          >
            <span class="text-muted-foreground">{{ hop.index + 1 }}</span>
            <span>{{ hop.method }}</span>
            <span class="min-w-0 flex-1"><TruncatedText :text="hop.url" /></span>
            <span>→</span>
            <Tooltip v-if="hop.status > 0">
              <TooltipTrigger as-child>
                <Badge data :variant="statusClass(hop.status)"
                  >{{ hop.status }} {{ hop.statusText }}</Badge
                >
              </TooltipTrigger>
              <TooltipContent>{{ statusHint(hop.status) }}</TooltipContent>
            </Tooltip>
            <Badge data v-else variant="error" data-testid="http-timeline-hop-failed-chip">
              {{ hop.error || 'failed' }}
            </Badge>
          </div>

          <div class="relative h-3 overflow-hidden rounded-kira-sm bg-field">
            <div class="absolute inset-y-0 flex min-w-0.5" :style="hopBarStyle(hop)">
              <span
                v-for="(seg, i) in hopSegments(hop)"
                :key="i"
                class="h-full"
                :class="seg.bgClass"
                :style="{ width: `${seg.widthPct}%` }"
              />
            </div>
          </div>

          <div class="flex flex-wrap gap-1.5 text-kira-sm">
            <template v-for="seg in PHASE_SEGMENTS" :key="seg.key">
              <Tooltip v-if="!hop[seg.key]">
                <TooltipTrigger as-child>
                  <span
                    class="text-muted-foreground data-[present=false]:text-subtle"
                    :data-testid="`http-timeline-phase-${seg.key}`"
                    data-present="false"
                  >
                    {{ t(seg.labelKey) }}
                    —
                  </span>
                </TooltipTrigger>
                <TooltipContent>{{ phaseTooltip(hop, seg.key) }}</TooltipContent>
              </Tooltip>
              <span
                v-else
                class="text-muted-foreground data-[present=false]:text-subtle"
                :data-testid="`http-timeline-phase-${seg.key}`"
                data-present="true"
              >
                {{ t(seg.labelKey) }} {{ formatDuration(hop[seg.key]!.durationMs) }}
              </span>
            </template>
          </div>

          <div
            v-if="hopNotes(hop).length > 0"
            class="flex flex-col gap-0.5 text-kira-sm text-subtle"
          >
            <div v-for="n in hopNotes(hop)" :key="n.testid" :data-testid="n.testid">{{
              n.text
            }}</div>
          </div>

          <Disclosure
            v-if="hopHeaders(hop).length > 0"
            class="mt-0.5"
            :label="t('studio.http.timeline.responseHeaders.label')"
            trigger-class="text-kira-sm text-subtle"
          >
            <KeyValueList class="py-0.5 text-kira-sm font-data">
              <template v-for="(h, i) in hopHeaders(hop)" :key="i">
                <dt>{{ h.name }}</dt>
                <dd class="wrap-anywhere">{{ h.value }}</dd>
              </template>
            </KeyValueList>
          </Disclosure>
        </Card>
      </div>
    </template>

    <EmptyState
      v-else-if="response"
      data-testid="http-timeline-empty"
      icon="watch"
      :title="t('studio.http.timeline.none.empty')"
    />
    <EmptyState
      v-else-if="rt?.status === 'error'"
      data-testid="http-timeline-empty"
      :icon="ICON.warning"
      :title="t('studio.http.timeline.failed.empty')"
    />
    <EmptyState v-else icon="arrow-right" :title="t('studio.common.responseEmpty.empty')" />
  </div>
</template>
