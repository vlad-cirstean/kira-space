import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import { type ComputedRef, computed, type Ref, ref } from 'vue';
import type { BeautifyMode, BeautifyResult } from '../../beautify';
import { reasonText } from './reasonText';

export interface EditBufferOptions {
  /** The stored value the buffer seeds from and Revert returns to. */
  original: () => string;
  /** `null` when this surface has no lossless formatter for the value in hand. */
  beautifier: () => ((text: string, mode: BeautifyMode) => BeautifyResult) | null;
  /** A surface that also has to un-stage something on Revert (the grid's pending-change set). */
  onRevert?: () => void;
}

export interface EditBuffer {
  doc: Ref<string>;
  isDirty: ComputedRef<boolean>;
  byteLabel: ComputedRef<string>;
  formatted: ComputedRef<'none' | 'indented' | 'compact'>;
  beautifyFailure: Ref<string | null>;
  canBeautify: ComputedRef<boolean>;
  applyBeautify(mode: BeautifyMode): void;
  /** Discards the buffer and un-stages whatever `onRevert` owns — the Revert button's action. */
  reset(): void;
  /**
   * Re-seeds the buffer from `original()` unconditionally, with no `onRevert` side effect — a
   * fresh publication overwriting stale display state, not a user action undoing a pending edit.
   * Deduping against "did the target actually change" is the caller's job: only it knows what
   * "the same target" means (a cell's identity, a document's `_id`), so it must call this only
   * when that identity check says so — see CellEditorView.vue's own `cellKey`-keyed watch.
   */
  reseed(): void;
  /** P24 D20's write-guard, unchanged: only writes when the candidate differs. */
  writeDoc(next: string): boolean;
}

const byteEncoder = new TextEncoder();

/**
 * The dirty/beautify/bytes/revert state machine shared by the cell editor and the document
 * editor (P27 D26) — one implementation instead of two hand-rolled copies drifting apart.
 */
export function useEditBuffer(opts: EditBufferOptions): EditBuffer {
  const doc = ref(opts.original());
  const formattedMode = ref<BeautifyMode>('indented');
  const formattedForDoc = ref<string | null>(null);
  const beautifyFailure = ref<string | null>(null);

  const isDirty = computed(() => doc.value !== opts.original());
  const byteLabel = computed(() => formatBytes(byteEncoder.encode(doc.value).length));
  const canBeautify = computed(() => opts.beautifier() !== null);
  // `formattedForDoc` reads 'indented'/'compact' only while the buffer still equals exactly what
  // applyBeautify last produced, and falls back to 'none' the instant doc changes by any other
  // path (a hand edit, a fresh cell, Reset) — never a flag every doc-writing call site must
  // remember to clear.
  const formatted = computed<'none' | 'indented' | 'compact'>(() =>
    formattedForDoc.value !== null && formattedForDoc.value === doc.value
      ? formattedMode.value
      : 'none',
  );

  function writeDoc(next: string): boolean {
    if (next === doc.value) return false;
    doc.value = next;
    return true;
  }

  // Acts on the buffer, not the stored value — beautifying a hand-edit formats the edit instead
  // of discarding it (P24 D21).
  function applyBeautify(mode: BeautifyMode): void {
    const fn = opts.beautifier();
    if (!fn) return;
    const result = fn(doc.value, mode);
    if (result.ok) {
      formattedMode.value = mode;
      formattedForDoc.value = result.text;
      doc.value = result.text;
      beautifyFailure.value = null;
    } else {
      // A failed beautify leaves the buffer and `formatted` alone.
      beautifyFailure.value = result.reason
        ? reasonText(result.reason)
        : t('studio.beautify.failed.error');
    }
  }

  function reset(): void {
    doc.value = opts.original();
    formattedForDoc.value = null;
    beautifyFailure.value = null;
    opts.onRevert?.();
  }

  function reseed(): void {
    doc.value = opts.original();
    formattedForDoc.value = null;
    beautifyFailure.value = null;
  }

  return {
    doc,
    isDirty,
    byteLabel,
    formatted,
    beautifyFailure,
    canBeautify,
    applyBeautify,
    reset,
    reseed,
    writeDoc,
  };
}
