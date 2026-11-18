<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { EdgeLabelRenderer, type EdgeProps } from '@vue-flow/core';
import { computed } from 'vue';
import { type EdgeData, NODE_H, type Point, RANK_SEP } from '../board/workflowGraph';

// A route between steps, grouped by target: toned by the results on it (all ok green, all not ok red,
// mixed neutral). Forward edges run orthogonal, jogging in the gap between ranks. A loop edge runs in
// the left gutter of the canvas and enters the target from above, dashed, with its budget.
const props = defineProps<EdgeProps<EdgeData>>();
const d = computed(() => props.data as EdgeData);
const TONE_STROKE = {
  ok: 'stroke-ok',
  fail: 'stroke-error',
  neutral: 'stroke-muted-foreground',
};
const TONE_MARKER = { ok: 'ade-arrow-ok', fail: 'ade-arrow-fail', neutral: 'ade-arrow-neutral' };
const RADIUS = 10;
const DROP = 16;
const LOOP_DROP = 44;
const LOOP_RISE = 18;
const HALF_GAP = RANK_SEP / 2;

/** Polyline through the corners, each corner smoothed to at most `RADIUS`. */
function smoothCorners(pts: Point[]): string {
  const p = pts.filter(
    (q, i) => i === 0 || q.x !== (pts[i - 1] as Point).x || q.y !== (pts[i - 1] as Point).y,
  );
  let out = `M ${(p[0] as Point).x} ${(p[0] as Point).y}`;
  for (let i = 1; i < p.length - 1; i += 1) {
    const a = p[i - 1] as Point;
    const b = p[i] as Point;
    const c = p[i + 1] as Point;
    const r = Math.min(
      RADIUS,
      Math.hypot(b.x - a.x, b.y - a.y) / 2,
      Math.hypot(c.x - b.x, c.y - b.y) / 2,
    );
    const u = (from: Point, to: Point): Point => {
      const len = Math.hypot(to.x - from.x, to.y - from.y) || 1;
      return { x: (to.x - from.x) / len, y: (to.y - from.y) / len };
    };
    const ab = u(a, b);
    const bc = u(b, c);
    out += ` L ${b.x - ab.x * r} ${b.y - ab.y * r} Q ${b.x} ${b.y} ${b.x + bc.x * r} ${b.y + bc.y * r}`;
  }
  const last = p[p.length - 1] as Point;
  return `${out} L ${last.x} ${last.y}`;
}

const geometry = computed(() => {
  const { sourceX: sx, sourceY: sy, targetX: tx, targetY: ty } = props;
  if (d.value.loop && d.value.laneX !== undefined) {
    const lx = d.value.laneX;
    const jog = sy + LOOP_DROP + Math.min(d.value.lane ?? 0, 3) * 4;
    const above = ty - LOOP_RISE;
    const pts = [
      { x: sx, y: sy },
      { x: sx, y: jog },
      { x: lx, y: jog },
      { x: lx, y: above },
      { x: tx, y: above },
      { x: tx, y: ty },
    ];
    return { path: smoothCorners(pts), x: lx + 6, y: jog, anchor: 'left' };
  }
  const via = d.value.points ?? [];
  const pts: Point[] = [{ x: sx, y: sy }];
  let jog = sy + HALF_GAP;
  let x = sx;
  for (const v of via) {
    pts.push({ x, y: jog }, { x: v.x, y: jog });
    x = v.x;
    jog = v.y + NODE_H / 2 + HALF_GAP;
  }
  pts.push({ x, y: ty - HALF_GAP }, { x: tx, y: ty - HALF_GAP }, { x: tx, y: ty });
  return { path: smoothCorners(pts), x: sx, y: sy + DROP, anchor: 'centre' };
});
const label = computed(() => `${d.value.results.join(', ')}${d.value.loop ? ' ↩' : ''}`);
</script>

<template>
  <g
    data-testid="ade-wf-edge"
    :data-tone="d.tone"
    :data-loop="d.loop"
    :data-spine="d.spine"
    :data-selected="d.selected"
    :data-results="d.results.join(',')"
  >
    <path
      :d="geometry.path"
      class="fill-none"
      :class="[
        TONE_STROKE[d.tone],
        d.loop ? '[stroke-dasharray:6_4]' : '',
        d.selected ? 'stroke-[3px]' : 'stroke-2',
      ]"
      :marker-end="`url(#${TONE_MARKER[d.tone]})`"
    />
    <path :d="geometry.path" class="fill-none stroke-transparent stroke-[16px]" />
  </g>
  <EdgeLabelRenderer v-if="!d.fixed">
    <Badge
      variant="chip"
      class="nodrag nopan pointer-events-auto absolute border border-border bg-bg text-muted-foreground"
      :style="{
        transform: `${geometry.anchor === 'left' ? 'translate(0, -50%)' : 'translate(-50%, -50%)'} translate(${geometry.x}px, ${geometry.y}px)`,
      }"
      data-testid="ade-wf-edge-label"
    >
      {{ label }}
    </Badge>
  </EdgeLabelRenderer>
</template>
