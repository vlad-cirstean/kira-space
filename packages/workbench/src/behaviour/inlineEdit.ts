import { type ComputedRef, computed, type Ref, ref, watch } from 'vue';
import { useAsyncAction } from './asyncAction';

export interface InlineEditOptions {
  /** The stored value. A push replaces the text only while no unsaved edit is held. */
  source: () => string;
  /** Persist `text`. A rejection keeps the field open with the error. */
  commit: (text: string) => void | Promise<void>;
  /** Esc, or a commit that changed nothing. */
  cancel?: () => void;
  /** A blank commit is valid (a settings field). Default false: blank cancels, so a row name never
   *  becomes empty. */
  allowEmpty?: boolean;
  /** The field closes after its first commit or cancel (a row name the parent swaps out), so the
   *  blur that follows an Enter is ignored. */
  once?: boolean;
}

/** Commit-on-Enter-or-blur, cancel-on-Esc text editing, shared by the inline row name and the
 *  settings-like fields. Enter ignores IME composition. */
export function useInlineEdit(opts: InlineEditOptions): {
  text: Ref<string>;
  error: ComputedRef<string>;
  pending: ComputedRef<boolean>;
  onInput: (value: string | number) => void;
  onCommit: () => Promise<void>;
  onCancel: () => void;
  onKeydown: (event: KeyboardEvent) => void;
} {
  const text = ref(opts.source());
  const action = useAsyncAction(async (next: string) => {
    await opts.commit(next);
  });
  const pending = action.pending;
  const error = computed(() => action.error.value ?? '');
  let dirty = false;
  let settled = false;

  watch(opts.source, (value) => {
    if (!dirty) text.value = value;
  });

  function onInput(value: string | number): void {
    text.value = String(value);
    dirty = true;
  }

  function onCancel(): void {
    if (settled) return;
    settled = opts.once === true;
    text.value = opts.source();
    action.reset();
    dirty = false;
    opts.cancel?.();
  }

  async function onCommit(): Promise<void> {
    if (pending.value || settled) return;
    const next = text.value.trim();
    if (next === opts.source() || (next === '' && !opts.allowEmpty)) {
      onCancel();
      return;
    }
    if (await action.run(next)) {
      settled = opts.once === true;
      dirty = false;
    }
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.isComposing) return;
    if (event.key === 'Enter') {
      event.preventDefault();
      void onCommit();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      onCancel();
    }
  }

  return { text, error, pending, onInput, onCommit, onCancel, onKeydown };
}
