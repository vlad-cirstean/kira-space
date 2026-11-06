<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { copyText } from '@workbench/behaviour/clipboard';
import { ref } from 'vue';

// Real-trackpad late-data capture (plan §8.3 step 5): Start, one hard two-finger flick, Stop. The
// JSON is copied to the clipboard.
const running = ref(false);
const summary = ref('');

function percentile(sorted: number[], q: number): number {
  return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * q))] ?? 0;
}

async function toggle(): Promise<void> {
  const trace = window.__kiraProtoTrace;
  if (!trace) return;
  if (!running.value) {
    trace.start();
    running.value = true;
    return;
  }
  const result = trace.stop();
  running.value = false;
  const moving = result.frames.filter((f) => f.pxPerFrame > 0).map((f) => f.gapPx);
  const sorted = [...moving].sort((a, b) => a - b);
  summary.value = `frames ${moving.length}, gap p95 ${percentile(sorted, 0.95).toFixed(0)} px, max ${(sorted.at(-1) ?? 0).toFixed(0)} px`;
  await copyText(JSON.stringify(result));
}
</script>

<template>
  <div
    class="fixed bottom-2 right-4 z-50 flex items-center gap-2 rounded-kira bg-elevated px-2 py-1 text-kira-md shadow-md"
    data-testid="trace-hud"
  >
    <Button size="kira" variant="toolbar" data-testid="trace-toggle" @click="toggle">
      {{ running ? 'Stop trace' : 'Start trace' }}
    </Button>
    <span class="text-fg-muted" data-testid="trace-summary">{{ summary }}</span>
  </div>
</template>
