import { CROCKFORD_ALPHABET, crockfordBase32 } from '@shared/domain/mask';
import { newUuid } from '@shared/ids';
import type { MessageKey } from '@theme/i18n';
import type { CellFormat } from './formats';

// Four values a developer routinely needs to type into a column by hand — pure and Vue-free.
// Never format-gated (that was the actual bug in the button this panel replaces): a generator
// writes text into the buffer, and the buffer does not care what format the value is being read
// as. "Now" is the one entry that is format-aware, because the right *text* for "the current
// moment" genuinely differs between an ISO timestamp and an epoch count.

// ULID's timestamp field: the 48-bit millisecond value written into a 50-bit, 10-character
// Crockford field, MSB-first — i.e. left-padded with two zero bits, not right-padded. Repeated
// division by 32, prepending each digit, produces exactly that: 10 base-32 digits (50 bits of
// room) can only ever need the top two digits for padding when the value fits in 48 bits, and
// division naturally leaves them as leading zeros rather than appending anything at the tail.
//
// This file used to carry its own byte-for-byte copy of mask.ts's crockfordBase32 (as toCrockford,
// used for the random half below) — reusing the shared one instead. Kept as a *separate* function
// from crockfordBase32 rather than folded into it: crockfordBase32's own trailing-pad branch pads
// a partial *last* group, which for this 50-bit, 10-character field would pad at the wrong end —
// ULID's timestamp is left-padded with 2 zero bits at the *top*, not right-padded — and would
// decode to a timestamp four times too large.
function encodeUlidTime(ms: number): string {
  let n = ms;
  let out = '';
  for (let i = 0; i < 10; i++) {
    const mod = n % 32;
    out = CROCKFORD_ALPHABET[mod] + out;
    n = (n - mod) / 32;
  }
  return out;
}

// 48-bit timestamp (10 Crockford chars) + 80 random bits (16 chars) = 26 chars, sortable by
// creation time unlike a v4 UUID — the second most-requested "give me an id" shape after UUID.
// The random half is an exact multiple of 5 bits (16 groups), so crockfordBase32's own
// trailing-pad branch never fires for it — byte-for-byte the same output the old local
// toCrockford produced.
function generateUlid(): string {
  const time = encodeUlidTime(Date.now());
  const random = crockfordBase32(crypto.getRandomValues(new Uint8Array(10)));
  return time + random;
}

function generateToken(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

function generateNow(format: CellFormat): string {
  const now = Date.now();
  if (format === 'epochSeconds') return String(Math.floor(now / 1000));
  if (format === 'epochMillis') return String(now);
  return new Date(now).toISOString();
}

export interface Generator {
  id: string;
  labelKey: MessageKey;
  hintKey: MessageKey;
  run(format: CellFormat): string;
}

export const GENERATORS: readonly Generator[] = [
  {
    id: 'uuid',
    labelKey: 'studio.cellEditor.generate.uuid.label',
    hintKey: 'studio.cellEditor.generate.uuid.description',
    run: () => newUuid(),
  },
  {
    id: 'ulid',
    labelKey: 'studio.cellEditor.generate.ulid.label',
    hintKey: 'studio.cellEditor.generate.ulid.description',
    run: generateUlid,
  },
  {
    id: 'token',
    labelKey: 'studio.cellEditor.generate.token.label',
    hintKey: 'studio.cellEditor.generate.token.description',
    run: generateToken,
  },
  {
    id: 'now',
    labelKey: 'common.now.status',
    hintKey: 'studio.cellEditor.generate.now.description',
    run: generateNow,
  },
];
