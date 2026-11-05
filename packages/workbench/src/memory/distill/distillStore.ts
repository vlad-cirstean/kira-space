import type { DistillMode, DistillResult, DistillStage } from '@shared/domain/memoryDistill';
import { newUuid } from '@shared/ids';
import { defineStore } from 'pinia';
import { ref } from 'vue';

/** Where a pasted selection came from: a file and its 1-based line range. */
export interface DistillOrigin {
  path: string;
  start: number;
  end: number;
}

export interface DistillRun {
  stage: DistillStage;
  done: number;
  total: number;
  costUsd: number;
}

// P285 Part 4: distill draft, run and result. Survives a module switch (the Memory module unmounts);
// the abort handle lives here so a remounted view can still cancel, like useModelDownloadsStore.
export const useDistillUiStore = defineStore('distillUi', () => {
  const source = ref<'text' | 'file'>('text');
  const text = ref('');
  const mode = ref<DistillMode>('facts');
  const path = ref('');
  const origin = ref<DistillOrigin | null>(null);
  const requestId = ref('');
  const progress = ref<DistillRun | null>(null);
  const result = ref<DistillResult | null>(null);
  // Where the result came from: a file path, `path:start-end` for a selection, empty for pasted text.
  const resultSource = ref('');
  const cancelled = ref(false);
  const selected = ref<Set<number>>(new Set());
  let controller: AbortController | undefined;

  function begin(): AbortSignal {
    controller = new AbortController();
    requestId.value = newUuid();
    progress.value = null;
    result.value = null;
    cancelled.value = false;
    return controller.signal;
  }
  function cancel(): void {
    cancelled.value = true;
    controller?.abort();
  }
  function end(): void {
    controller = undefined;
  }
  function setResult(value: DistillResult, from: string): void {
    result.value = value;
    resultSource.value = from;
    selected.value = new Set(value.lines.keys());
  }
  function openWith(args: { text: string; origin: DistillOrigin | null }): void {
    source.value = 'text';
    text.value = args.text;
    origin.value = args.origin;
    path.value = '';
    result.value = null;
    progress.value = null;
    cancelled.value = false;
  }

  return {
    source,
    text,
    mode,
    path,
    origin,
    requestId,
    progress,
    result,
    resultSource,
    cancelled,
    selected,
    begin,
    cancel,
    end,
    setResult,
    openWith,
  };
});
