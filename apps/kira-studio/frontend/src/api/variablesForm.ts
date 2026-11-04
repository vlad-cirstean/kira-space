import {
  type EnvEntry,
  type EnvParseError,
  type EnvRow,
  parseEnv,
  reconcileEnv,
  serializeEnv,
} from '@kira/api-core';
import type { ApiFileVariable } from '@shared/domain/apiConfig';
import type { FieldSpec } from '@workbench/config/configForm';

// Every leaf the variables dialog writes, in the file's own keys. A collection file shares the table:
// its name and description never change in the dialog, so they produce no edit. The variable list is
// written whole; keys not listed (a collection's items) are never sent.
export const FILE_FIELDS: readonly FieldSpec[] = [
  { path: ['name'] },
  { path: ['description'], unsetWhenEmpty: true },
  { path: ['color'], unsetWhenEmpty: true, unsetWhen: 'none' },
  { path: ['variables'], unsetWhenEmpty: true },
];

export type VariablesSection = 'environment' | 'variables' | 'yaml';

/** Pointers each section claims, for the problem strip. */
export const VARIABLES_SECTION_POINTERS = {
  environment: ['/name', '/description', '/color'],
  variables: ['/variables'],
} as const satisfies Record<Exclude<VariablesSection, 'yaml'>, readonly string[]>;

const SECRET_REF = /^\$\{secret:([^}]+)\}$/;

/** The secret name inside a whole-value `${secret:name}` reference, '' for anything else. */
export function secretNameOf(ref: string | undefined): string {
  return SECRET_REF.exec(ref ?? '')?.[1] ?? '';
}

export const secretRef = (name: string): string => `\${secret:${name}}`;

/** The file name without its kind suffix. */
export function fileStem(file: string): string {
  return file.replace(/\.(collection|environment)\.yaml$/, '');
}

/** A valid secret name for a variable: `<file stem>.<variable name>`, lowercase, 64 characters at most. */
export function variableSecretName(stem: string, variable: string): string {
  const slug = variable
    .toLowerCase()
    .replace(/[^a-z0-9_.-]+/g, '-')
    .replace(/^[^a-z0-9]+/, '');
  return `${stem}.${slug || 'variable'}`.slice(0, 64);
}

/** A row as the file holds it: empty keys dropped, and a secret never keeps a plain value. */
export function cleanVariable(v: ApiFileVariable): ApiFileVariable {
  const out: ApiFileVariable = { name: v.name };
  if (v.secret) out.secret = v.secret;
  else if (v.value) out.value = v.value;
  if (v.description) out.description = v.description;
  return out;
}

function toEnvRows(vars: readonly ApiFileVariable[]): EnvRow[] {
  return vars.map((v, i) => ({
    id: String(i),
    name: v.name,
    value: v.secret ? '' : (v.value ?? ''),
    isSecret: !!v.secret,
    description: v.description ?? '',
  }));
}

export const serializeVariables = (vars: readonly ApiFileVariable[]): string =>
  serializeEnv(toEnvRows(vars));

export interface BulkResult {
  variables: ApiFileVariable[];
  /** Values typed on a secret's line, by the secret's name. */
  typed: Record<string, string>;
  summary: { added: number; updated: number; removed: number };
  error: EnvParseError | null;
}

/** The rows `text` describes, matched to `base` by name (the Nth repeat of a name to the Nth row of
 *  that name). A line on a secret's row keeps the reference; a new line is a plain variable. */
export function applyBulk(base: readonly ApiFileVariable[], text: string): BulkResult {
  const parsed = parseEnv(text);
  if (parsed.error) {
    return {
      variables: [...base],
      typed: {},
      summary: { added: 0, updated: 0, removed: 0 },
      error: parsed.error,
    };
  }
  const diff = reconcileEnv(toEnvRows(base), parsed.entries);
  const pools = new Map<string, ApiFileVariable[]>();
  for (const v of base) pools.set(v.name, [...(pools.get(v.name) ?? []), v]);
  const typed: Record<string, string> = {};
  const variables = parsed.entries.map((e: EnvEntry): ApiFileVariable => {
    const match = pools.get(e.name)?.shift();
    if (match?.secret) {
      if (e.hasValue) typed[secretNameOf(match.secret)] = e.value;
      return cleanVariable({ ...match, description: e.description });
    }
    return cleanVariable({ name: e.name, value: e.value, description: e.description });
  });
  return {
    variables,
    typed,
    summary: {
      added: diff.added.length,
      updated: diff.updated.length,
      removed: diff.removed.length,
    },
    error: null,
  };
}
