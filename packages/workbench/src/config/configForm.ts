// Pure helpers of useConfigFormDraft: leaf access by key path and the leaf diff that turns a form
// model change into file edits.

export interface FieldSpec {
  path: readonly string[];
  /** An empty value removes the key instead of writing it. */
  unsetWhenEmpty?: boolean;
  /** Exclusive partner keys: a non-empty write of this field unsets each of them. */
  clears?: readonly (readonly string[])[];
  /** A mapping whose children have their own specs: this spec only adds or removes the mapping. */
  presence?: boolean;
}

/** One file edit, in the file's own key vocabulary (the server's `configfs.Edit`). */
export interface FormEdit {
  path: string[];
  value?: unknown;
  unset: boolean;
}

/** A file problem; `path` is a JSON pointer. */
export interface FormProblem {
  line: number;
  col?: number;
  path?: string;
  message: string;
}

export interface FormResult<V> {
  text: string;
  value: V | null;
  problem: FormProblem | null;
}

function getLeaf(root: unknown, path: readonly string[]): unknown {
  let at = root;
  for (const key of path) {
    if (at === null || typeof at !== 'object') return undefined;
    at = (at as Record<string, unknown>)[key];
  }
  return at;
}

function isEmptyValue(v: unknown): boolean {
  return v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0);
}

function sameValue(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (a === null || b === null || typeof a !== 'object' || typeof b !== 'object') return false;
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  const ka = Object.keys(a);
  const kb = Object.keys(b);
  if (ka.length !== kb.length) return false;
  return ka.every((k) =>
    sameValue((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k]),
  );
}

/** The edits that take `base` to `model` over the field table. Fields the table does not name are
 *  never sent, so keys the form does not know stay in the file untouched. */
export function diffLeaves(
  fields: readonly FieldSpec[],
  base: unknown,
  model: unknown,
): FormEdit[] {
  const edits: FormEdit[] = [];
  for (const spec of fields) {
    const next = getLeaf(model, spec.path);
    if (spec.presence) {
      const had = !isEmptyValue(getLeaf(base, spec.path));
      const has = !isEmptyValue(next);
      if (had === has) continue;
      edits.push(
        has
          ? { path: [...spec.path], value: next, unset: false }
          : { path: [...spec.path], unset: true },
      );
      continue;
    }
    if (sameValue(next, getLeaf(base, spec.path))) continue;
    const empty = isEmptyValue(next);
    if (empty && (spec.unsetWhenEmpty || next === undefined || next === null)) {
      edits.push({ path: [...spec.path], unset: true });
      continue;
    }
    edits.push({ path: [...spec.path], value: next, unset: false });
    if (!empty) {
      for (const partner of spec.clears ?? []) edits.push({ path: [...partner], unset: true });
    }
  }
  return edits;
}

/** True when a problem pointer is `pointer` or sits under it. */
export function pointerCovers(problemPath: string | undefined, pointer: string): boolean {
  if (!problemPath) return false;
  return problemPath === pointer || problemPath.startsWith(`${pointer}/`);
}
