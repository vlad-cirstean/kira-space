import type { FieldSpec } from '@workbench/config/configForm';

// Every leaf the connection form writes, in the file's own keys. A value equal to its default is
// removed, as the file format omits it. Keys not listed (options, tree, schema_ddl) are never sent.
export const CONNECTION_FIELDS: readonly FieldSpec[] = [
  { path: ['name'] },
  { path: ['kind'] },
  { path: ['color'], unsetWhen: 'none' },
  { path: ['read_only'], unsetWhen: false },
  { path: ['mode'], unsetWhen: 'fields' },
  { path: ['host'], unsetWhenEmpty: true },
  { path: ['port'], unsetWhen: 0 },
  { path: ['database'], unsetWhenEmpty: true },
  { path: ['username'], unsetWhenEmpty: true },
  { path: ['password'], unsetWhenEmpty: true },
  { path: ['uri'], unsetWhenEmpty: true },
  { path: ['auto_explain'], unsetWhen: false },
  { path: ['throttle_per_sec'], unsetWhen: 0 },
  { path: ['preconnect'], presence: true },
  { path: ['preconnect', 'command'] },
  { path: ['preconnect', 'sidecar'], unsetWhen: false },
  { path: ['preconnect', 'delay_sec'], unsetWhen: 0 },
  { path: ['mcp'], presence: true },
  { path: ['mcp', 'enabled'], unsetWhen: false },
  { path: ['mcp', 'description'], unsetWhenEmpty: true },
  { path: ['mcp', 'read'], unsetWhen: 'allow' },
  { path: ['mcp', 'write'], unsetWhen: 'prompt' },
  { path: ['mcp', 'ddl'], unsetWhen: 'deny' },
  { path: ['mcp', 'masked'], unsetWhen: 'inherit' },
  { path: ['mcp', 'auto_explain'], unsetWhen: true },
  { path: ['mcp', 'review_results'], unsetWhen: 'inherit' },
  { path: ['mcp', 'review'], presence: true },
  { path: ['mcp', 'review', 'read'], unsetWhen: 'inherit' },
  { path: ['mcp', 'review', 'write'], unsetWhen: 'inherit' },
  { path: ['mcp', 'review', 'ddl'], unsetWhen: 'inherit' },
  { path: ['mcp', 'review', 'masked'], unsetWhen: 'inherit' },
  { path: ['mcp', 'statement_timeout_sec'], unsetWhen: 10 },
  { path: ['masks'], unsetWhenEmpty: true },
];

export type ConnectionSection = 'general' | 'advanced' | 'preconnect' | 'mcp' | 'privacy' | 'yaml';

/** Pointers each tab claims, for the error count on its label and the problem strip. */
export const CONNECTION_TAB_POINTERS = {
  general: [
    '/name',
    '/kind',
    '/color',
    '/mode',
    '/host',
    '/port',
    '/database',
    '/username',
    '/password',
    '/uri',
  ],
  advanced: ['/read_only', '/auto_explain', '/throttle_per_sec'],
  preconnect: ['/preconnect'],
  mcp: ['/mcp'],
  privacy: ['/masks'],
} as const satisfies Record<Exclude<ConnectionSection, 'yaml'>, readonly string[]>;

const SECRET_REF = /^\$\{secret:([^}]+)\}$/;

/** The secret name inside a whole-value `${secret:name}` reference, '' for anything else. */
export function secretNameOf(ref: string | undefined): string {
  return SECRET_REF.exec(ref ?? '')?.[1] ?? '';
}

export const secretRef = (name: string): string => `\${secret:${name}}`;
