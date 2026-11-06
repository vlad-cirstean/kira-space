/**
 * The one place the frontend mints ids: UUIDv7 for keys and machine identifiers, prefixed nanoids
 * for handles the user touches. Mirrors internal/ids (check-conventions N13 keeps the registry equal).
 */
import { customAlphabet } from 'nanoid';
import { v4, v7 } from 'uuid';

/** Crockford base32 lowercase: no i, l, o, u look-alikes. */
const ALPHABET = '0123456789abcdefghjkmnpqrstvwxyz';

/** Registered nanoid prefixes and their body lengths. */
export const PREFIXES = {
  wt: 10,
  km: 12,
  kg: 12,
  shot: 12,
  tui: 12,
  sbx: 12,
  tmp: 12,
  dtc: 12,
  dfl: 12,
  frn: 12,
  stp: 12,
  exm: 12,
} as const;

export type IdPrefix = keyof typeof PREFIXES;

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const generators = new Map<IdPrefix, () => string>();

/** A UUIDv7 string. Works on a plain-HTTP origin, where `crypto.randomUUID` is missing. */
export function newUuid(): string {
  return v7();
}

/** A random UUIDv4 for values written into user data (cell editor generator); no embedded time. */
export function newUserUuidV4(): string {
  return v4();
}

/** True for a canonical UUID of any version, so legacy v4 ids pass. */
export function isUuid(s: string): boolean {
  return UUID_RE.test(s);
}

/** `<prefix>_<nanoid>` with the registered body length. */
export function newId(prefix: IdPrefix): string {
  let gen = generators.get(prefix);
  if (!gen) {
    gen = customAlphabet(ALPHABET, PREFIXES[prefix]);
    generators.set(prefix, gen);
  }
  return `${prefix}_${gen()}`;
}

/** True when `s` is exactly one id of `prefix`. */
export function isId(prefix: IdPrefix, s: string): boolean {
  const head = `${prefix}_`;
  if (!s.startsWith(head) || s.length !== head.length + PREFIXES[prefix]) return false;
  for (const ch of s.slice(head.length)) if (!ALPHABET.includes(ch)) return false;
  return true;
}

/** The part after the first underscore, or `s` when there is none. */
export function idBody(s: string): string {
  const i = s.indexOf('_');
  return i < 0 ? s : s.slice(i + 1);
}
