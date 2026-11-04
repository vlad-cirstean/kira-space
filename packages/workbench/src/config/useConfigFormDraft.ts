import { useDebounceFn } from '@vueuse/core';
import { computed, type MaybeRefOrGetter, ref, toValue, watch } from 'vue';
import { useAsyncAction } from '../behaviour/asyncAction';
import { TIMING } from '../behaviour/timing';
import { errorText } from '../bridge/codedError';
import {
  diffLeaves,
  type FieldSpec,
  type FormEdit,
  type FormProblem,
  type FormResult,
  pointerCovers,
} from './configForm';

// One draft text per dialog is the only state that saves. The form is a view of it: a model
// change becomes node edits on the server (comments and key order outside the edit stay), a YAML
// edit re-decodes into the model. Save is the module's own SaveFile(text, baseHash), so conflict
// handling and the projection path stay as they are. Keys the field table does not name are never
// sent, so they pass through untouched.

export interface ConfigFileData {
  text: string;
  hash: string;
}

export interface ConfigFormDraftOptions<V extends object> {
  /** The file as the query holds it; undefined while loading. */
  file: MaybeRefOrGetter<ConfigFileData | undefined>;
  /** Applies edits to `text` and decodes the result. No edits: decode only. */
  form: (text: string, edits: FormEdit[]) => Promise<FormResult<V>>;
  /** Writes the whole text built on `baseHash`; resolves the new hash. */
  save: (text: string, baseHash: string) => Promise<{ hash: string }>;
  refetch: () => Promise<ConfigFileData | undefined>;
  fields: readonly FieldSpec[];
  /** Runs after the file write succeeds, e.g. the write-only secret calls. */
  afterSave?: () => Promise<void>;
}

export function useConfigFormDraft<V extends object>(opts: ConfigFormDraftOptions<V>) {
  const tab = ref<'form' | 'yaml'>('form');
  const draft = ref('');
  const baseHash = ref('');
  const dirty = ref(false);
  const model = ref<V | null>(null);
  const problem = ref<FormProblem | null>(null);
  const conflict = ref(false);

  let baseline: V | null = null;
  let chain: Promise<void> = Promise.resolve();
  let decodeSeq = 0;

  function queue(work: () => Promise<void>): Promise<void> {
    chain = chain.then(work, work);
    return chain;
  }

  function adopt(result: FormResult<V>): void {
    problem.value = result.problem;
    if (result.value) {
      baseline = structuredClone(result.value);
      model.value = structuredClone(result.value);
    }
  }

  // Re-reads the model from the draft text (YAML edits, load, tab switch).
  function decode(): Promise<void> {
    return queue(async () => {
      const mine = ++decodeSeq;
      const text = draft.value;
      try {
        const result = await opts.form(text, []);
        if (mine === decodeSeq && draft.value === text) adopt(result);
      } catch (err) {
        problem.value = { line: 0, message: errorText(err) };
      }
    });
  }

  // Sends the model's changed leaves; the draft becomes the edited text.
  function applyModel(): Promise<void> {
    return queue(async () => {
      const edits = diffLeaves(opts.fields, baseline, model.value);
      if (edits.length === 0) return;
      try {
        const result = await opts.form(draft.value, edits);
        draft.value = result.text;
        dirty.value = true;
        problem.value = result.problem;
        if (result.value) baseline = structuredClone(result.value);
      } catch (err) {
        problem.value = { line: 0, message: errorText(err) };
      }
    });
  }

  const scheduleApply = useDebounceFn(applyModel, TIMING.preview);
  const scheduleDecode = useDebounceFn(decode, TIMING.preview);

  watch(model, () => void scheduleApply(), { deep: true });

  const writer = useAsyncAction(async (text: string) => {
    try {
      const next = await opts.save(text, baseHash.value);
      conflict.value = false;
      baseHash.value = next.hash;
      if (draft.value === text) dirty.value = false;
      await opts.afterSave?.();
    } catch (err) {
      conflict.value = (err as { code?: string }).code === 'E_CONFLICT';
      throw err;
    }
  });
  const saving = writer.pending;
  const saveError = computed(() => writer.error.value ?? '');

  function resetTo(data: ConfigFileData | undefined): Promise<void> {
    draft.value = data?.text ?? '';
    baseHash.value = data?.hash ?? '';
    dirty.value = false;
    writer.error.value = null;
    conflict.value = false;
    return decode();
  }

  watch(
    () => toValue(opts.file),
    (data) => {
      if (!data || dirty.value) return;
      void resetTo(data);
    },
    { immediate: true },
  );

  function onYamlInput(text: string): void {
    draft.value = text;
    dirty.value = true;
    void scheduleDecode();
  }

  async function showTab(next: 'form' | 'yaml'): Promise<void> {
    tab.value = next;
    if (next === 'form') await decode();
  }

  async function saveNow(): Promise<void> {
    if (saving.value) return;
    if (tab.value === 'form') {
      await applyModel();
      if (problem.value) return;
    }
    if (!dirty.value) return;
    await writer.run(draft.value);
  }

  const canSave = computed(
    () => dirty.value && !saving.value && !(tab.value === 'form' && problem.value),
  );
  const formDisabled = computed(() => problem.value !== null);

  function discard(): Promise<void> {
    return resetTo(toValue(opts.file));
  }

  async function reload(): Promise<void> {
    await resetTo(await opts.refetch());
  }

  async function overwrite(): Promise<void> {
    const fresh = await opts.refetch();
    if (!fresh) return;
    baseHash.value = fresh.hash;
    conflict.value = false;
    dirty.value = true;
    await saveNow();
  }

  /** The problem message when it names `pointer` (a JSON pointer) or a key under it. */
  function errorFor(pointer: string): string | undefined {
    const p = problem.value;
    return p && pointerCovers(p.path, pointer) ? p.message : undefined;
  }

  /** A problem no field claims, for the Alert on top of the form. */
  function unmatched(claimed: readonly string[]): FormProblem | null {
    const p = problem.value;
    if (!p) return null;
    return claimed.some((c) => pointerCovers(p.path, c)) ? null : p;
  }

  return {
    tab,
    draft,
    baseHash,
    dirty,
    model,
    problem,
    saveError,
    conflict,
    saving,
    canSave,
    formDisabled,
    showTab,
    onYamlInput,
    saveNow,
    discard,
    reload,
    overwrite,
    errorFor,
    unmatched,
  };
}
