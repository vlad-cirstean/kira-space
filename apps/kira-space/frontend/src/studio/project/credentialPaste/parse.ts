import type { ConnectionKind } from '@shared/domain/connection';
import { AWS_STYLE_KINDS, FILE_KINDS } from '@shared/domain/connection';
import { t } from '@theme/i18n';
import { parse as shellParse } from 'shell-quote';

export type PasteField =
  | 'host'
  | 'port'
  | 'database'
  | 'username'
  | 'password'
  | 'region'
  | 'profile'
  | 'authSource';

export interface PasteCandidate {
  value: string;
  guessed: boolean;
  /** The key as written (`PGPASSWORD`, `-u`), shown so the user sees why a value matched. */
  label: string;
}

/** Never carries a pasted value: recognised-but-unusable labels only. */
interface PasteNote {
  label: string;
  reason: string;
}

export interface PasteResult {
  fields: Partial<Record<PasteField, PasteCandidate[]>>;
  notes: PasteNote[];
  /** Lines with no recognised label; counted, never echoed (they may hold secrets). */
  unrecognised: number;
  tooLong: boolean;
}

export const MAX_PASTE_BYTES = 8 * 1024;

type Slot = PasteField | 'awsSecret' | 'schema';

const PREFIX =
  '(?:pg|(?:postgres|postgresql|mysql|mariadb|mongo|mongodb|redis|kafka|clickhouse|db|database|spring[_.]datasource|sasl|aws)[_.\\- ]?)';

// Longest-first matters only for readable alternation; the separator that must follow the key
// forces backtracking to the right alias anyway.
const WORDS: readonly (readonly [Slot, readonly string[]])[] = [
  ['awsSecret', ['access key id', 'access key', 'secret access key', 'session token']],
  [
    'username',
    [
      'username',
      'user name',
      'user id',
      'userid',
      'user',
      'uid',
      'login',
      'role',
      'account',
      'principal',
    ],
  ],
  [
    'password',
    [
      'password',
      'passwd',
      'pass phrase',
      'passphrase',
      'pass',
      'pwd',
      'secret',
      'token',
      'root[_.\\- ]?password',
      'rediscli[_.\\- ]?auth',
    ],
  ],
  [
    'host',
    [
      'hostname',
      'host name',
      'host',
      'server',
      'address',
      'addr',
      'endpoint',
      'data source',
      'datasource',
      'bootstrap servers',
      'brokers',
      'broker',
    ],
  ],
  ['port', ['port']],
  ['schema', ['schema']],
  [
    'database',
    [
      'database',
      'dbname',
      'db name',
      'db',
      'initial catalog',
      'catalog',
      'service name',
      'service',
      'sid',
    ],
  ],
  ['region', ['default region', 'region']],
  ['profile', ['profile']],
  ['authSource', ['authsource', 'auth source', 'authenticationdatabase', 'auth db']],
];

function wordSource(word: string): string {
  return word.includes('[') ? word : word.replace(/ /g, '[ _.\\-]?');
}

const SLOT_RE: readonly (readonly [Slot, RegExp])[] = WORDS.map(([slot, words]) => [
  slot,
  new RegExp(`^(?:${PREFIX})?(?:${words.map(wordSource).join('|')})$`, 'i'),
]);
// MongoDB's docker image names: MONGO_INITDB_ROOT_USERNAME / _PASSWORD.
const INITDB_RE = /^mongo_initdb_root_(username|password)$/i;

const ALL_KEYS = [
  'mongo[_.]initdb[_.]root[_.](?:username|password)',
  ...WORDS.flatMap(([, words]) => words.map(wordSource)),
]
  .sort((a, b) => b.length - a.length)
  .join('|');

// Group 1 key, group 2 explicit separator, group 3 whitespace-only separator.
const HIT_SOURCE = `(?<![\\w.\\-])((?:${PREFIX})?(?:${ALL_KEYS}))(?:[ \\t]*(=>|->|:=|:|=)[ \\t]*|([ \\t]+))`;

function classify(key: string): Slot | null {
  const init = INITDB_RE.exec(key);
  if (init) return init[1].toLowerCase() === 'username' ? 'username' : 'password';
  for (const [slot, re] of SLOT_RE) if (re.test(key)) return slot;
  return null;
}

interface Raw {
  slot: Slot;
  value: string;
  guessed: boolean;
  label: string;
}

interface Hit {
  index: number;
  key: string;
  slot: Slot;
  explicit: boolean;
  valueStart: number;
}

function* hits(line: string, from: number): Generator<Hit> {
  const re = new RegExp(HIT_SOURCE, 'gi');
  re.lastIndex = from;
  for (let m = re.exec(line); m; m = re.exec(line)) {
    const slot = classify(m[1]);
    if (slot) {
      yield {
        index: m.index,
        key: m[1],
        slot,
        explicit: m[2] !== undefined,
        valueStart: m.index + m[0].length,
      };
    }
    // Resume right after the key so an overlapping label inside this separator span is still seen.
    re.lastIndex = m.index + m[1].length;
  }
}

// How the text before `index` delimits a pair: `;`/`,`, a tab or 2+ spaces, or the line start.
function delimiterBefore(line: string, index: number): 'start' | 'delimiter' | 'space' | null {
  const before = line.slice(0, index);
  if (before.trim() === '') return 'start';
  if (/[;,][ \t]*$/.test(before) || /(?:\t| {2,})$/.test(before)) return 'delimiter';
  if (/ $/.test(before)) return 'space';
  return null;
}

function validHit(line: string, h: Hit): boolean {
  const d = delimiterBefore(line, h.index);
  if (d === null) return false;
  if (d === 'space') return h.explicit;
  return true;
}

function readQuoted(
  line: string,
  start: number,
): { value: string; end: number; terminated: boolean } {
  const quote = line[start];
  let out = '';
  for (let i = start + 1; i < line.length; i++) {
    const c = line[i];
    if (quote === '"' && c === '\\' && (line[i + 1] === '"' || line[i + 1] === '\\')) {
      out += line[i + 1];
      i++;
    } else if (c === quote) {
      return { value: out, end: i + 1, terminated: true };
    } else {
      out += c;
    }
  }
  return { value: out, end: line.length, terminated: false };
}

function scanLine(line: string, out: Raw[]): boolean {
  let pos = 0;
  let found = false;
  const next = (from: number): Hit | null => {
    for (const h of hits(line, from)) if (validHit(line, h)) return h;
    return null;
  };
  for (let h = next(pos); h; h = next(pos)) {
    let value: string;
    let guessed = false;
    const first = line[h.valueStart];
    if (first === '"' || first === "'" || first === '`') {
      const q = readQuoted(line, h.valueStart);
      value = q.value;
      guessed = !q.terminated;
      pos = q.end;
    } else {
      let end = line.length;
      for (const c of hits(line, h.valueStart)) {
        if (validHit(line, c)) {
          end = c.index;
          break;
        }
      }
      value = line.slice(h.valueStart, end).trim().replace(/[;,]$/, '').trim();
      pos = Math.max(end, h.valueStart);
    }
    found = true;
    if (value !== '') out.push({ slot: h.slot, value, guessed, label: h.key });
    // Guard: a zero-width advance would loop forever.
    if (pos <= h.index) pos = h.index + h.key.length;
  }
  return found;
}

// ---- CLI flags ------------------------------------------------------------------------------

interface Flag {
  slot: PasteField;
  /** Value must be attached (`-pSecret`, `--password=x`); a detached one is a prompt. */
  attachedOnly?: boolean;
  /** Numeric means port, otherwise password, flagged guessed (bare `-p`). */
  portOrPassword?: boolean;
}

type FlagTable = Record<string, Flag>;

const PG_FLAGS: FlagTable = {
  '-U': { slot: 'username' },
  '--username': { slot: 'username' },
  '-h': { slot: 'host' },
  '--host': { slot: 'host' },
  '-p': { slot: 'port' },
  '--port': { slot: 'port' },
  '-d': { slot: 'database' },
  '--dbname': { slot: 'database' },
};
const MYSQL_FLAGS: FlagTable = {
  '-u': { slot: 'username' },
  '--user': { slot: 'username' },
  '-p': { slot: 'password', attachedOnly: true },
  '--password': { slot: 'password', attachedOnly: true },
  '-h': { slot: 'host' },
  '--host': { slot: 'host' },
  '-P': { slot: 'port' },
  '--port': { slot: 'port' },
  '-D': { slot: 'database' },
  '--database': { slot: 'database' },
};
const MONGO_FLAGS: FlagTable = {
  '-u': { slot: 'username' },
  '--username': { slot: 'username' },
  '-p': { slot: 'password' },
  '--password': { slot: 'password' },
  '--host': { slot: 'host' },
  '--port': { slot: 'port' },
  '--authenticationDatabase': { slot: 'authSource' },
};
const REDIS_FLAGS: FlagTable = {
  '-h': { slot: 'host' },
  '-p': { slot: 'port' },
  '-a': { slot: 'password' },
  '--pass': { slot: 'password' },
  '--user': { slot: 'username' },
  '-n': { slot: 'database' },
};
const CLICKHOUSE_FLAGS: FlagTable = {
  '-h': { slot: 'host' },
  '--host': { slot: 'host' },
  '--port': { slot: 'port' },
  '-u': { slot: 'username' },
  '--user': { slot: 'username' },
  '--password': { slot: 'password' },
  '-d': { slot: 'database' },
  '--database': { slot: 'database' },
};
const KAFKA_FLAGS: FlagTable = {
  '-b': { slot: 'host' },
  '--bootstrap-server': { slot: 'host' },
};
const DEFAULT_FLAGS: FlagTable = {
  '-u': { slot: 'username' },
  '--user': { slot: 'username' },
  '--username': { slot: 'username' },
  '-h': { slot: 'host' },
  '--host': { slot: 'host' },
  '-P': { slot: 'port' },
  '--port': { slot: 'port' },
  '-d': { slot: 'database' },
  '--database': { slot: 'database' },
  '--password': { slot: 'password', attachedOnly: true },
  '-p': { slot: 'password', attachedOnly: true, portOrPassword: true },
};

interface Client {
  flags: FlagTable;
  /** Slots for the first bare positionals, in order. */
  positional?: readonly PasteField[];
  /** Flags whose value is not a credential but must not be read as a positional. */
  skip?: ReadonlySet<string>;
  /** `-X name=value` style librdkafka properties. */
  properties?: boolean;
}

const PSQL: Client = {
  flags: PG_FLAGS,
  positional: ['database', 'username'],
  skip: new Set(['-c', '-f', '-o', '-v', '-F', '-P', '-T', '-L', '-e']),
};
const MYSQL: Client = { flags: MYSQL_FLAGS, positional: ['database'], skip: new Set(['-e']) };
const MONGO: Client = { flags: MONGO_FLAGS, skip: new Set(['--eval']) };
const CLIENTS: Record<string, Client> = {
  psql: PSQL,
  pg_dump: PSQL,
  mysql: MYSQL,
  mariadb: MYSQL,
  mysqldump: MYSQL,
  mysqlsh: MYSQL,
  mongosh: MONGO,
  mongo: MONGO,
  'redis-cli': { flags: REDIS_FLAGS },
  'clickhouse-client': { flags: CLICKHOUSE_FLAGS },
  clickhouse: { flags: CLICKHOUSE_FLAGS },
  kcat: { flags: KAFKA_FLAGS, properties: true },
  'kafka-console-consumer': { flags: KAFKA_FLAGS },
};
const DEFAULT_CLIENT: Client = { flags: DEFAULT_FLAGS };

const LEADING_ASSIGNS = /^(?:[A-Za-z_]\w*=(?:"[^"]*"|'[^']*'|\S*)[ \t]+)+/;

function clientOf(line: string): { prefix: string; rest: string; name: string | null } | null {
  const assign = LEADING_ASSIGNS.exec(line);
  const prefix = assign ? assign[0] : '';
  const rest = line.slice(prefix.length);
  const word = /^\S+/.exec(rest)?.[0] ?? '';
  const name = (word.split('/').pop() ?? '').toLowerCase();
  if (Object.hasOwn(CLIENTS, name)) return { prefix, rest, name };
  if (rest.startsWith('-')) return { prefix, rest, name: null };
  return null;
}

function tokenize(text: string): string[] {
  // Return the literal `$NAME` so nothing is ever substituted from an environment.
  const entries = shellParse(text, (key: string) => `$${key}`);
  const tokens: string[] = [];
  for (const e of entries) {
    if (typeof e === 'string') tokens.push(e);
    else if ('pattern' in e) tokens.push(e.pattern);
    else break;
  }
  return tokens;
}

function splitFlag(t: string): { flag: string; attached: string | undefined } {
  if (t.startsWith('--')) {
    const eq = t.indexOf('=');
    return eq < 0
      ? { flag: t, attached: undefined }
      : { flag: t.slice(0, eq), attached: t.slice(eq + 1) };
  }
  return { flag: t.slice(0, 2), attached: t.length > 2 ? t.slice(2) : undefined };
}

function flagValue(
  spec: Flag,
  attached: string | undefined,
  tokens: string[],
  i: number,
): string | undefined {
  if (attached !== undefined) return attached;
  return spec.attachedOnly ? undefined : tokens[i + 1];
}

function emitFlag(spec: Flag, flag: string, value: string, out: Raw[]): void {
  if (spec.portOrPassword) {
    const isPort = /^\d+$/.test(value);
    out.push({ slot: isPort ? 'port' : 'password', value, guessed: !isPort, label: flag });
  } else {
    out.push({ slot: spec.slot, value, guessed: false, label: flag });
  }
}

// Handles one flag token; returns how many following tokens it consumed.
function scanFlag(client: Client, tokens: string[], i: number, out: Raw[]): number {
  const { flag, attached } = splitFlag(tokens[i]);
  if (flag === '-X' && client.properties) {
    scanLine(attached ?? tokens[i + 1] ?? '', out);
    return attached === undefined ? 1 : 0;
  }
  const spec = client.flags[flag];
  if (!spec) return client.skip?.has(flag) && attached === undefined ? 1 : 0;
  const value = flagValue(spec, attached, tokens, i);
  if (value !== undefined && value !== '') emitFlag(spec, flag, value, out);
  return attached === undefined && !spec.attachedOnly ? 1 : 0;
}

function scanCli(rest: string, name: string | null, out: Raw[]): void {
  const client =
    (name !== null && Object.hasOwn(CLIENTS, name) ? CLIENTS[name] : undefined) ?? DEFAULT_CLIENT;
  const tokens = tokenize(rest);
  let positional = 0;
  for (let i = name ? 1 : 0; i < tokens.length; i++) {
    const token = tokens[i];
    if (token.startsWith('-') && token !== '-') {
      i += scanFlag(client, tokens, i, out);
      continue;
    }
    const slot = client.positional?.[positional++];
    if (slot)
      out.push({
        slot,
        value: token,
        guessed: true,
        label: t('studio.project.credentialPaste.label.positional.label'),
      });
  }
}

// ---- Normalise, post-process ---------------------------------------------------------------

function normalise(text: string): string[] {
  return text
    .replace(/^\uFEFF/, '')
    .replace(/\r\n?/g, '\n')
    .replace(/[\u00A0\u1680\u2000-\u200A\u202F\u205F\u3000]/g, ' ')
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '' && !l.startsWith('#') && !l.startsWith('//'))
    .map((l) => l.replace(/^(?:export|set)[ \t]+/i, ''));
}

const URI_LIKE = /^[a-z][a-z0-9+.-]*:\/\//i;

function addCandidate(fields: PasteResult['fields'], slot: PasteField, c: PasteCandidate): void {
  const list = fields[slot] ?? [];
  const dup = list.find((x) => x.value === c.value);
  if (dup) {
    dup.guessed = dup.guessed && c.guessed;
    return;
  }
  list.push(c);
  fields[slot] = list;
}

// Why a field cannot be used for this kind, or null when it can.
function unusableReason(slot: PasteField, kind: ConnectionKind, value: string): string | null {
  if (AWS_STYLE_KINDS.has(kind)) {
    return slot === 'region' || slot === 'profile'
      ? null
      : t('studio.project.credentialPaste.reason.awsNoSuchField.error');
  }
  if (slot === 'region' || slot === 'profile')
    return t('studio.project.credentialPaste.reason.awsOnly.error');
  if (slot === 'authSource' && kind !== 'mongodb')
    return t('studio.project.credentialPaste.reason.authSourceMongo.error');
  if (slot === 'database' && kind === 'kafka')
    return t('studio.project.credentialPaste.reason.kafkaNoDatabase.error');
  if ((slot === 'host' || slot === 'database') && URI_LIKE.test(value)) {
    return t('studio.project.credentialPaste.reason.uri.error');
  }
  if (slot === 'database' && kind === 'redis' && !/^\d+$/.test(value)) {
    return t('studio.project.credentialPaste.reason.redisDatabase.error');
  }
  return null;
}

function build(raws: Raw[], kind: ConnectionKind, unrecognised: number): PasteResult {
  const fields: PasteResult['fields'] = {};
  const notes: PasteNote[] = [];
  const add = (slot: PasteField, c: PasteCandidate): void => addCandidate(fields, slot, c);
  const addPort = (value: string, guessed: boolean, label: string): void => {
    const n = Number(value);
    if (!/^\d+$/.test(value) || n < 1 || n > 65535) {
      notes.push({ label, reason: t('studio.project.credentialPaste.reason.port.error') });
      return;
    }
    add('port', { value: String(n), guessed, label });
  };
  const addHost = (raw: string, guessed: boolean, label: string): void => {
    let value = raw;
    if (kind === 'kafka' && value.includes(',')) {
      const parts = value
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean);
      value = parts[0] ?? '';
      if (parts.length > 1) {
        notes.push({
          label,
          reason: t('studio.project.credentialPaste.reason.extraBrokers.error'),
        });
      }
    }
    const m = /^\[([^\]]+)\](?::(\d+))?$/.exec(value) ?? /^([^:\s]+):(\d+)$/.exec(value);
    if (m) {
      value = m[1];
      if (m[2] !== undefined) addPort(m[2], guessed, label);
    }
    if (value !== '') add('host', { value, guessed, label });
  };

  for (const r of raws) {
    const { label, value } = r;
    if (r.slot === 'awsSecret') {
      notes.push({ label, reason: t('studio.project.credentialPaste.reason.awsKeys.error') });
      continue;
    }
    if (r.slot === 'schema') {
      notes.push({ label, reason: t('studio.project.credentialPaste.reason.noSchema.error') });
      continue;
    }
    if (FILE_KINDS.has(kind)) continue;
    const reason = unusableReason(r.slot, kind, value);
    if (reason) {
      notes.push({ label, reason });
      continue;
    }
    const guessed = r.guessed || (r.slot === 'database' && /^(?:service|sid)/i.test(label));
    if (r.slot === 'host') addHost(value, guessed, label);
    else if (r.slot === 'port') addPort(value, guessed, label);
    else add(r.slot, { value, guessed, label });
  }

  return { fields, notes, unrecognised, tooLong: false };
}

// D12: no labelled credentials: two whitespace-free lines (or tokens) are username then
// password, one token is a password. Anything else yields nothing.
function unlabelledFallback(lines: string[], kind: ConnectionKind, result: PasteResult): void {
  if (AWS_STYLE_KINDS.has(kind) || FILE_KINDS.has(kind)) return;
  if (Object.keys(result.fields).length > 0 || result.notes.length > 0) return;
  const tokens = lines.length === 1 ? lines[0].split(/[ \t]+/) : lines;
  const plain = tokens.every((t) => t !== '' && !/\s/.test(t));
  if (!plain || tokens.length === 0 || tokens.length > 2) return;
  const label = t('studio.project.credentialPaste.label.unlabelled.label');
  if (tokens.length === 2) {
    result.fields.username = [{ value: tokens[0], guessed: true, label }];
    result.fields.password = [{ value: tokens[1], guessed: true, label }];
  } else {
    result.fields.password = [{ value: tokens[0], guessed: true, label }];
  }
  result.unrecognised = 0;
}

export function parseCredentialPaste(text: string, kind: ConnectionKind): PasteResult {
  const empty: PasteResult = { fields: {}, notes: [], unrecognised: 0, tooLong: false };
  if (new Blob([text]).size > MAX_PASTE_BYTES) return { ...empty, tooLong: true };
  try {
    const lines = normalise(text);
    const raws: Raw[] = [];
    let unrecognised = 0;
    for (const line of lines) {
      const before = raws.length;
      const cli = clientOf(line);
      if (cli) {
        if (cli.prefix) scanLine(cli.prefix.trim(), raws);
        scanCli(cli.rest, cli.name, raws);
      } else {
        scanLine(line, raws);
      }
      if (raws.length === before) unrecognised++;
    }
    const result = build(raws, kind, unrecognised);
    unlabelledFallback(lines, kind, result);
    return result;
  } catch {
    // Total by contract: a malformed paste yields nothing, never an error carrying its text.
    return empty;
  }
}
