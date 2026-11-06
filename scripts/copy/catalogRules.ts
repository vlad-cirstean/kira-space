/**
 * P265 F3: catalog rules C1-C11 over the flattened message catalog (git show bd0c224688d9:docs/v2.2/plans/P265-text-catalog-i18n.md
 * section 3.2). Pure functions; `check-catalog.ts` is the CLI.
 */
import { baseCompile } from '@intlify/message-compiler';

export type RuleId = 'C1' | 'C2' | 'C3' | 'C4' | 'C5' | 'C6' | 'C7' | 'C8' | 'C9' | 'C10' | 'C11';

export interface Violation {
  key: string;
  rule: RuleId;
  message: string;
}

export interface DuplicateExemption {
  /** Exact keys or `*` globs (one segment per `*`). A duplicate group is covered when every key matches. */
  keys: string[];
  reason: string;
}

/** Proper nouns, product names, acronyms and key names that keep their capitals mid-sentence. */
export const PROPER_NOUNS: readonly string[] = [
  'Kira Studio',
  'Kira Space',
  'Kira',
  'Finder',
  'Get Info',
  'Open with',
  'Change All',
  'Bash',
  'Apache',
  'Claude Code',
  'Claude',
  'OpenCode',
  'Git',
  'GitHub',
  'Azure',
  'DevOps',
  'Docker Desktop',
  'Docker',
  'Jira',
  'MCP',
  'DB',
  'AI',
  'SSO',
  'PII',
  'CDATA',
  'DOCTYPE',
  'PATH',
  'MB',
  'Max',
  'SQL',
  'JSON',
  'YAML',
  'CSV',
  'TSV',
  'HTTP',
  'GET',
  'HTTPS',
  'gRPC',
  'URL',
  'URI',
  'SHA',
  'PR',
  'ID',
  'API',
  'DDL',
  'TLS',
  'SSL',
  'AWS',
  'S3',
  'SQS',
  'Kafka',
  'Redis',
  'MongoDB',
  'PostgreSQL',
  'MySQL',
  'MariaDB',
  'SQLite',
  'ClickHouse',
  'Kibana',
  'Elasticsearch',
  'KQL',
  'Lucene',
  'Discover',
  'macOS',
  'Mac',
  'iPhone',
  'Android',
  'Wi-Fi',
  'EXPLAIN',
  'SELECT',
  'WITH',
  'ANALYZE',
  'LIMIT',
  'INSERT',
  'UPDATE',
  'DELETE',
  'CREATE',
  'ALTER',
  'DROP',
  'TRUNCATE',
  'NULL',
  'ADE',
  'CPU',
  'UTC',
  'EJSON',
  'ObjectId',
  'Monaco',
  'curl',
  'Postman',
  'DataGrip',
  'Colima',
  'Enter',
  'Esc',
  'Tab',
  'Shift',
  'Cmd',
  'Ctrl',
  'Alt',
  'Option',
  'Return',
  // Additions beyond the plan list: widely-known acronyms that sentence-case prose needs.
  'PDF',
  'PNG',
  'SSH',
  'DNS',
  'IP',
  'UI',
  'OS',
  'UUID',
  'XML',
  'HTML',
  'CSS',
  'CLI',
  'SDK',
  'REST',
  'LSP',
  'ANSI',
  'GPU',
  'RAM',
];

/** Duplicate and near-duplicate groups that stay on purpose. Every entry names why. */
export const ALLOWED_DUPLICATES: readonly DuplicateExemption[] = [];

export const ROLES: ReadonlySet<string> = new Set([
  'label',
  'tooltip',
  'aria',
  'placeholder',
  'title',
  'option',
  'status',
  'announce',
  'confirm',
  'body',
  'hint',
  'description',
  'emptyHint',
  'error',
  'empty',
  'loading',
  'example',
]);

export const SENTENCE_ROLES: ReadonlySet<string> = new Set([
  'body',
  'hint',
  'description',
  'emptyHint',
  'error',
]);

type Role = string;

const ROLE_OVERRIDES: readonly [RegExp, Role][] = [
  [/^format\./, 'format'],
  [/^common\.state\.loading$/, 'loading'],
  [/^common\.state\./, 'status'],
  [/^common\.copy\.failed$/, 'error'],
  [/^common\.copy\./, 'announce'],
  [/^common\.key\./, 'label'],
  [/^common\.product\./, 'label'],
  [/^common\.module\.[a-zA-Z0-9]+$/, 'label'],
  [/^common\.confirm\.irreversible$/, 'body'],
];

const SEGMENT_RE = /^[a-z][a-zA-Z0-9]*$/;
const PLACEHOLDER_NAME_RE = /^[a-z][a-zA-Z0-9]*$/;
const MINOR_WORDS = new Set([
  'a',
  'an',
  'the',
  'and',
  'or',
  'of',
  'in',
  'on',
  'to',
  'for',
  'with',
  'as',
  'at',
  'by',
  'but',
  'from',
  'vs',
]);

const NEW_SENTENCE_GAP = /(?:[.!?…:]|—)[\s“"‘(]*$/;
const PH = '\u0001';
const PHRASE = '\u0002';

export type Catalog = ReadonlyMap<string, string>;

export interface FlattenResult {
  catalog: Map<string, string>;
  violations: Violation[];
}

export function flattenCatalog(root: unknown): FlattenResult {
  const catalog = new Map<string, string>();
  const violations: Violation[] = [];
  const visit = (node: unknown, path: string): void => {
    if (typeof node === 'string') {
      catalog.set(path, node);
    } else if (node !== null && typeof node === 'object' && !Array.isArray(node)) {
      for (const [k, v] of Object.entries(node)) visit(v, path === '' ? k : `${path}.${k}`);
    } else {
      violations.push({
        key: path,
        rule: 'C2',
        message: 'leaf must be a string (no array, number or null)',
      });
    }
  };
  visit(root, '');
  return { catalog, violations };
}

export function roleOf(key: string): Role | undefined {
  for (const [re, role] of ROLE_OVERRIDES) if (re.test(key)) return role;
  const last = key.slice(key.lastIndexOf('.') + 1);
  return ROLES.has(last) ? last : undefined;
}

function isConfirmTitle(key: string): boolean {
  const seg = key.split('.');
  return seg[seg.length - 1] === 'title' && (seg[seg.length - 2] ?? '').endsWith('Confirm');
}

/** Plural branches of a message (vue-i18n `a | b | c`). One element when the message has no `|`. */
export function branches(value: string): string[] {
  return value.split('|').map((b) => b.trim());
}

const LITERAL_RE = /\{'[^']*'\}/g;
const LITERAL_TEST = /\{'[^']*'\}/;
const PLACEHOLDER_RE = /\{([^{}]*)\}/g;

/** Value with literal interpolations (`{'@'}`) removed and placeholders replaced by `with`. */
export function maskPlaceholders(value: string, withText = ''): string {
  return value.replace(LITERAL_RE, withText).replace(PLACEHOLDER_RE, withText);
}

/** Named placeholders of a message, `{name}` form only. */
export function placeholdersOf(value: string): Set<string> {
  const out = new Set<string>();
  const stripped = value.replace(LITERAL_RE, '');
  for (const m of stripped.matchAll(PLACEHOLDER_RE)) out.add((m[1] ?? '').trim());
  return out;
}

export function isPlural(value: string): boolean {
  return value.includes('|');
}

// C1
export function ruleC1(key: string, value: string): Violation[] {
  const errors: string[] = [];
  try {
    baseCompile(value, { onError: (e) => errors.push(e.message) });
  } catch (e) {
    errors.push((e as Error).message);
  }
  const masked = value.replace(LITERAL_RE, '');
  let depth = 0;
  let nested = false;
  for (const ch of masked) {
    if (ch === '{') depth++;
    else if (ch === '}') depth--;
    if (depth < 0 || depth > 1) nested = true;
  }
  if (depth !== 0 || nested) errors.push('unbalanced braces');
  if (/@(?!:|\.)/.test(masked)) errors.push("literal @ must be written {'@'}");
  return [...new Set(errors)].map((message) => ({ key, rule: 'C1' as const, message }));
}

// C2
export function ruleC2(key: string): Violation[] {
  const out: Violation[] = [];
  const seg = key.split('.');
  if (seg.length < 3 || seg.length > 6) {
    out.push({ key, rule: 'C2', message: `depth ${seg.length}, expected 3-6 segments` });
  }
  for (const s of seg) {
    if (!SEGMENT_RE.test(s))
      out.push({ key, rule: 'C2', message: `segment “${s}” is not camelCase` });
  }
  if (roleOf(key) === undefined) {
    out.push({ key, rule: 'C2', message: `leaf is not a role (${[...ROLES].join(', ')})` });
  }
  return out;
}

function normaliseWhitespace(v: string): string {
  return v.replace(/\s+/g, ' ').trim();
}

/** C4 normal form: case, trailing punctuation (a trailing `…` is kept), quotes, placeholders and spacing erased. */
export function nearForm(value: string): string {
  return value
    .toLowerCase()
    .replace(/[“”„«»]/g, '"')
    .replace(/[‘’]/g, "'")
    .replace(/\{[^{}]*\}/g, '{}')
    .replace(/\.{3}/g, '…')
    .replace(/[.:\s]+$/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function globToRegExp(glob: string): RegExp {
  const src = glob
    .split('.')
    .map((s) => (s === '*' ? '[^.]+' : s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
    .join('\\.');
  return new RegExp(`^${src}$`);
}

export function isCovered(
  keys: readonly string[],
  exemptions: readonly DuplicateExemption[],
): boolean {
  return exemptions.some((ex) => {
    const res = ex.keys.map(globToRegExp);
    return keys.every((k) => res.some((re) => re.test(k)));
  });
}

function groupBy(entries: Iterable<[string, string]>, norm: (v: string) => string): string[][] {
  const groups = new Map<string, string[]>();
  for (const [k, v] of entries) {
    const n = norm(v);
    if (n === '') continue;
    const g = groups.get(n);
    if (g) g.push(k);
    else groups.set(n, [k]);
  }
  return [...groups.values()].filter((g) => g.length > 1);
}

/** `backend.*` is read by Go as its own consumer; it never duplicates against the frontend. */
function partition(keys: string[]): string[][] {
  const back = keys.filter((k) => k.startsWith('backend.'));
  const front = keys.filter((k) => !k.startsWith('backend.'));
  return [back, front].filter((g) => g.length > 1);
}

// C3 and C4
export function ruleC3C4(
  catalog: Catalog,
  exemptions: readonly DuplicateExemption[] = ALLOWED_DUPLICATES,
): Violation[] {
  const out: Violation[] = [];
  const entries = [...catalog.entries()];
  const seen = new Set<string>();
  for (const group of groupBy(entries, normaliseWhitespace).flatMap(partition)) {
    if (isCovered(group, exemptions)) continue;
    seen.add(group.join('\u0000'));
    const first = group[0] as string;
    for (const key of group.slice(1)) {
      out.push({ key, rule: 'C3', message: `same text as ${first}; use one key` });
    }
  }
  for (const group of groupBy(entries, nearForm).flatMap(partition)) {
    if (isCovered(group, exemptions)) continue;
    const distinct = new Set(group.map((k) => normaliseWhitespace(catalog.get(k) as string)));
    if (distinct.size < 2) continue;
    const first = group[0] as string;
    for (const key of group.slice(1)) {
      if (catalog.get(key) === catalog.get(first)) continue;
      out.push({ key, rule: 'C4', message: `near-duplicate of ${first}; unify the wording` });
    }
  }
  return out;
}

const BANNED: readonly [RegExp, string][] = [
  [/\.\.\./, 'use the single character …'],
  [/['"]/, 'straight quote; use “ ” and ’'],
  [/\(s\)/, '(s) plural; use a plural message'],
  [/ & /, '& in prose; use and'],
  [/\be\.g\./i, 'e.g.; use for example'],
  [/\bOK\b/, 'OK; use the verb'],
  [/\bPreferences\b/i, 'Preferences; use Settings'],
  [/\bColor\b|\bcolor\b/, 'Color; use Colour'],
  [/\bcanceled\b/i, 'canceled; use cancelled'],
  [/\bcannot\b/i, 'cannot; use can’t'],
  [/\bcould not\b/i, 'could not; use couldn’t'],
  [/\bfailed to\b/i, 'failed to; use Couldn’t <verb>'],
  [/\bunable to\b/i, 'unable to; use can’t'],
  [/\bpick\b/i, 'Pick; use Choose or Select'],
  [/\btry again\b/i, 'Try again; use Retry'],
  [/\breload\b/i, 'Reload; use Refresh'],
  [/^Checkout\b/, 'Checkout as a verb; use Check out'],
  [/\brepo\b/i, 'repo; use repository'],
  [/\bdirectory\b/i, 'directory; use folder'],
  [/\b(?:Id|id)\b/, 'Id; use ID'],
  [/^Error:/, 'Error: prefix'],
  [/^\+ /, '+ prefix'],
  [/Nothing to show/, 'Nothing to show; use No <items>'],
];

// C5
export function ruleC5(key: string, value: string): Violation[] {
  if (roleOf(key) === 'example') return [];
  const out: Violation[] = [];
  const text = maskPlaceholders(value)
    .replace(/\bworking directory\b/gi, '')
    .replace(/\bhome directory\b/gi, '')
    .replace(/\bcherry-pick\b/gi, '');
  for (const branch of branches(text)) {
    for (const [re, message] of BANNED) {
      if (re.test(branch)) out.push({ key, rule: 'C5', message });
    }
  }
  return out;
}

const WORD_RE = new RegExp(`[\\p{L}][\\p{L}\\p{N}’'-]*|${PH}|${PHRASE}`, 'gu');

function stripPossessive(word: string): string {
  return word.replace(/[’']s$/, '');
}

function listWord(word: string, lowerList: ReadonlySet<string>): boolean {
  return lowerList.has(word) || lowerList.has(stripPossessive(word));
}

const PHRASES = PROPER_NOUNS.filter((p) => p.includes(' ')).sort((a, b) => b.length - a.length);
const SINGLES: ReadonlySet<string> = new Set(PROPER_NOUNS.filter((p) => !p.includes(' ')));

function wordProblems(text: string, titleCase: boolean): string[] {
  let masked = maskPlaceholders(text, PH);
  for (const phrase of PHRASES) masked = masked.split(phrase).join(PHRASE);
  const problems: string[] = [];
  let prevEnd = 0;
  let first = true;
  for (const m of masked.matchAll(WORD_RE)) {
    const word = m[0];
    const index = m.index ?? 0;
    const startsSentence = first || NEW_SENTENCE_GAP.test(masked.slice(prevEnd, index));
    prevEnd = index + word.length;
    first = false;
    if (word === PH || word === PHRASE) continue;
    const upper = /^\p{Lu}/u.test(word);
    const known = listWord(word, SINGLES);
    if (titleCase) {
      const minor = MINOR_WORDS.has(word.toLowerCase());
      if (!upper && !known && (startsSentence || !minor))
        problems.push(`“${word}” needs a capital`);
      continue;
    }
    if (startsSentence) {
      if (!upper && !known) problems.push(`sentence starts with lowercase “${word}”`);
    } else if (upper && !known) {
      problems.push(
        `“${word}” capitalised mid-sentence (sentence case; add proper nouns to PROPER_NOUNS)`,
      );
    }
  }
  return problems;
}

// C6
export function ruleC6(key: string, value: string): Violation[] {
  const role = roleOf(key);
  if (role === 'example' || role === 'format') return [];
  const titleCase = key.startsWith('backend.menu.');
  const out: Violation[] = [];
  for (const branch of branches(value)) {
    for (const message of wordProblems(branch, titleCase)) out.push({ key, rule: 'C6', message });
  }
  return out;
}

function punctuationProblem(key: string, role: Role, text: string): string | undefined {
  if (isConfirmTitle(key)) return text.endsWith('?') ? undefined : 'confirm title must end with ?';
  if (SENTENCE_ROLES.has(role)) {
    const ok = text.endsWith('.') || (role === 'error' && text.endsWith('}'));
    return ok ? undefined : `${role} is a sentence and must end with .`;
  }
  if (role === 'loading') return text.endsWith('…') ? undefined : 'loading text must end with …';
  if (text.endsWith('.')) return `${role} must not end with a period`;
  if (text.endsWith('?')) return `${role} must not end with ?`;
  if (role === 'placeholder' && text.endsWith('…')) return 'placeholder must not end with …';
  return undefined;
}

// C7
export function ruleC7(key: string, value: string): Violation[] {
  const role = roleOf(key);
  if (role === undefined || role === 'example' || role === 'format') return [];
  const out: Violation[] = [];
  for (const text of branches(value)) {
    if (text === '') continue;
    const message = punctuationProblem(key, role, text);
    if (message) out.push({ key, rule: 'C7', message });
  }
  return out;
}

// C8
export function ruleC8(key: string, value: string): Violation[] {
  const out: Violation[] = [];
  const push = (message: string) => out.push({ key, rule: 'C8', message });
  if (key.startsWith('backend.')) {
    if (value.includes('|')) push('backend messages have no plural');
    if (/@[:.]/.test(value)) push('backend messages have no linked message');
    if (LITERAL_TEST.test(value)) push('backend messages have no literal interpolation');
    for (const name of placeholdersOf(value)) {
      if (!PLACEHOLDER_NAME_RE.test(name))
        push(`backend placeholder {${name}} must be a named placeholder`);
    }
    return out;
  }
  if (!value.includes('|')) return out;
  const parts = branches(value);
  if (parts.length < 2 || parts.length > 3)
    push(`plural has ${parts.length} branches, expected 2 or 3`);
  if (!/\{count\}/.test(value)) push('plural must use {count}');
  if (parts.some((p) => p === '')) push('plural has an empty branch');
  return out;
}

// C9
export function ruleC9(key: string, value: string): Violation[] {
  const out: Violation[] = [];
  for (const name of placeholdersOf(value)) {
    if (!PLACEHOLDER_NAME_RE.test(name)) {
      out.push({ key, rule: 'C9', message: `placeholder {${name}} is not camelCase` });
    }
  }
  if (roleOf(key) !== 'example' && /<\/?[A-Za-z][^>]*>/.test(value)) {
    out.push({ key, rule: 'C9', message: 'no HTML tags in a message; use CatalogText slots' });
  }
  return out;
}

// C11
export function ruleC11(key: string, value: string): Violation[] {
  return value.trim() === '' ? [{ key, rule: 'C11', message: 'empty value' }] : [];
}

export interface UsedKeys {
  /** Quoted key literals found in sources. */
  literals: ReadonlySet<string>;
  /** Template-literal key prefixes (`studio.x.${kind}.label` gives `studio.x.`). */
  prefixes: readonly string[];
}

// C10
export function ruleC10(catalog: Catalog, used: UsedKeys): Violation[] {
  const out: Violation[] = [];
  for (const key of catalog.keys()) {
    if (used.literals.has(key)) continue;
    if (used.prefixes.some((p) => key.startsWith(p))) continue;
    out.push({ key, rule: 'C10', message: 'unused key' });
  }
  return out;
}

export interface CheckOptions {
  used?: UsedKeys;
  exemptions?: readonly DuplicateExemption[];
}

export function checkCatalog(root: unknown, options: CheckOptions = {}): Violation[] {
  const { catalog, violations } = flattenCatalog(root);
  const out = [...violations];
  for (const [key, value] of catalog) {
    out.push(
      ...ruleC1(key, value),
      ...ruleC2(key),
      ...ruleC5(key, value),
      ...ruleC6(key, value),
      ...ruleC7(key, value),
      ...ruleC8(key, value),
      ...ruleC9(key, value),
      ...ruleC11(key, value),
    );
  }
  out.push(...ruleC3C4(catalog, options.exemptions ?? ALLOWED_DUPLICATES));
  if (options.used) out.push(...ruleC10(catalog, options.used));
  return out;
}
