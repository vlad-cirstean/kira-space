<script setup lang="ts">
import AttributeTooltip from '@theme/components/AttributeTooltip.vue';
import { ref } from 'vue';

interface TipRequest {
  /** Viewport-relative to the grid root, in CSS px. */
  rect: { left: number; top: number; width: number; height: number };
  attrs: Record<string, string>;
}

const props = defineProps<{ container: HTMLElement | null }>();
const proxy = ref<HTMLElement | null>(null);

// The canvas has no DOM per cell, so a transparent proxy sits over the hovered cell and carries
// the same data-kira-tip attributes the SlickGrid header cells carry. AttributeTooltip resolves
// the hovered `[data-kira-tip]` from a bubbling pointermove, so the host replays one at the proxy
// (pointer-events none keeps clicks on the canvas) and one at the container to dismiss.
function show(request: TipRequest): void {
  const el = proxy.value;
  if (!el) return;
  for (const name of el.getAttributeNames()) {
    if (name.startsWith('data-kira-tip') || name === 'aria-label') el.removeAttribute(name);
  }
  for (const [name, value] of Object.entries(request.attrs)) el.setAttribute(name, value);
  const { left, top, width, height } = request.rect;
  Object.assign(el.style, {
    left: `${left}px`,
    top: `${top}px`,
    width: `${width}px`,
    height: `${height}px`,
    display: 'block',
  });
  el.dispatchEvent(new PointerEvent('pointermove', { bubbles: true }));
}

function hide(): void {
  const el = proxy.value;
  if (!el || el.style.display === 'none') return;
  props.container?.dispatchEvent(new PointerEvent('pointermove', { bubbles: true }));
  el.style.display = 'none';
}

defineExpose({ show, hide });
</script>

<template>
  <AttributeTooltip :container="container" />
  <div
    ref="proxy"
    class="pointer-events-none absolute"
    style="display: none"
    data-testid="grid-tip-proxy"
  />
</template>
