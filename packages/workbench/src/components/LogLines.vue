<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import { cardVariants } from '@theme/components/ui/card';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { computed, useTemplateRef } from 'vue';
import { useFollowTail } from '../behaviour/followTail';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '../behaviour/virtualRows';
import { type AnsiSegment, parseAnsi } from '../lib/ansi';

// The one log viewer (run logs, worktree setup, container logs): virtualized, ANSI-coloured,
// follows the tail until the reader scrolls up. Text selects (body is select-none); rows outside the
// window are unmounted, so a whole-log copy goes through the caller's data (CopyLogButton).
// Callers own loading and fetching; `testid` keeps each caller's own locator ids. The `line` slot replaces a line's content (match highlighting).
export interface LogLine {
  id: string | number;
  text: string;
  stream: string;
}

const props = defineProps<{
  lines: readonly LogLine[];
  testid: string;
  error?: string;
  truncated?: boolean;
  maxHeight?: string;
  /** Wrap long lines (default); false keeps one row per line and scrolls sideways. */
  wrap?: boolean;
}>();

const scrollEl = useTemplateRef<HTMLElement>('scrollEl');
const ESTIMATED_LINE_HEIGHT = 18;
const { virtualizer, virtualItems, totalSize, onScroll } = useVirtualRows({
  count: () => props.lines.length,
  rowHeight: () => ESTIMATED_LINE_HEIGHT,
  scrollElement: scrollEl,
});
// An explicit `v-model:follow` toggles following; left unbound, scrolling up pauses it and scrolling
// back to the bottom resumes it.
const follow = defineModel<boolean | undefined>('follow', { default: undefined });
const { atBottom, jumpToLatest } = useFollowTail(scrollEl, () => props.lines.length, { follow });

/** Scroll a line into view (the virtual row may not be mounted yet). */
function scrollToIndex(index: number): void {
  virtualizer.value.scrollToIndex(index, { align: 'center' });
}

defineExpose({ atBottom, jumpToLatest, scrollToIndex });

const style = computed(() => (props.maxHeight ? { maxHeight: props.maxHeight } : undefined));

function measure(el: unknown): void {
  if (el instanceof Element) virtualizer.value.measureElement(el);
}

// ANSI parse once per line object: the slot and the default row both ask, and every scroll or push
// re-renders the visible rows.
const parsed = new WeakMap<LogLine, AnsiSegment[]>();
function segmentsOf(index: number): AnsiSegment[] {
  const line = props.lines[index];
  if (!line) return [];
  let segments = parsed.get(line);
  if (!segments) {
    segments = parseAnsi(line.text.replace(/\n$/, ''));
    parsed.set(line, segments);
  }
  return segments;
}

function segmentStyle(seg: AnsiSegment): Record<string, string> | undefined {
  if (!seg.color && !seg.background && !seg.faint) return undefined;
  return {
    ...(seg.color && { color: seg.color }),
    ...(seg.background && { backgroundColor: seg.background }),
    ...(seg.faint && { opacity: '0.6' }),
  };
}

function streamClass(stream: string): string {
  if (stream === 'stderr') return 'text-error';
  return stream === 'event' ? 'text-muted-foreground' : 'text-fg';
}
</script>

<template>
  <div
    ref="scrollEl"
    :class="
      cn(
        cardVariants(),
        'min-h-0 flex-1 cursor-text select-text gap-0 overflow-auto font-data text-kira-sm leading-relaxed',
        !maxHeight && 'max-h-60',
      )
    "
    :style="style"
    :data-testid="testid"
    @scroll.passive="onScroll"
  >
    <InlineState v-if="error" kind="error" :data-testid="`${testid}-error`">{{
      error
    }}</InlineState>
    <template v-else>
      <div v-if="truncated" class="text-subtle">{{ t('common.truncated.status') }}</div>
      <div v-if="lines.length === 0" class="text-subtle">{{ t('common.noOutput.emptyHint') }}</div>
      <div v-else class="relative w-full shrink-0" :style="{ height: `${totalSize}px` }">
        <div
          v-for="item in virtualItems"
          :key="lines[item.index]?.id ?? item.key"
          :ref="measure"
          :data-index="item.index"
          :class="
            cn(
              VIRTUAL_ROW_CLASS,
              wrap === false ? 'whitespace-pre' : 'whitespace-pre-wrap break-words',
              streamClass(lines[item.index]?.stream ?? ''),
            )
          "
          :style="{ transform: `translateY(${item.start}px)` }"
          :data-testid="`${testid}-line`"
          :data-stream="lines[item.index]?.stream"
        >
          <slot
            name="line"
            :line="lines[item.index]"
            :segments="segmentsOf(item.index)"
            :index="item.index"
          >
            <span
              v-for="(seg, i) in segmentsOf(item.index)"
              :key="i"
              :class="seg.bold && 'font-bold'"
              :style="segmentStyle(seg)"
              >{{
                seg.text
              }}</span
            >
          </slot>
        </div>
      </div>
    </template>
  </div>
</template>
