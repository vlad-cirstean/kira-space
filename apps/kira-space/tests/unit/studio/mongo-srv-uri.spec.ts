// `canRoundTripToFields` must stay false for `mongodb+srv://` — nothing downstream can represent
// the `+srv` scheme (formatConnectionUri derives the scheme from `kind` alone, and the backend's
// buildURIFromFields hardcodes `mongodb://`), so allowing a +srv URI into fields mode silently
// discards the scheme with no way back, and SRV hosts have no port to satisfy fields mode's
// required-port validation. Plain `mongodb://` is unaffected and must keep round-tripping.
import { describe, expect, test } from 'bun:test';
import {
  canRoundTripToFields,
  formatConnectionUri,
  type ParsedUri,
  parseConnectionUri,
} from '@shared/domain/uri';

function mustParse(uri: string): ParsedUri {
  const parsed = parseConnectionUri(uri);
  if (!parsed) throw new Error(`expected ${uri} to parse`);
  return parsed;
}

describe('canRoundTripToFields (mongo +srv regression, round-2 finding 1)', () => {
  test('mongodb+srv:// stays URI-only (no port, scheme unrepresentable in fields mode)', () => {
    const parsed = mustParse(
      'mongodb+srv://user:pass@cluster0.xyz.mongodb.net/app?authSource=admin',
    );
    expect(canRoundTripToFields(parsed, 'mongodb')).toBe(false);
  });

  test('plain mongodb:// still round-trips to fields mode', () => {
    const parsed = mustParse('mongodb://user:pass@localhost:27017/app?authSource=admin');
    expect(canRoundTripToFields(parsed, 'mongodb')).toBe(true);
  });
});

type FormatInput = Parameters<typeof formatConnectionUri>[0];

// Only the fields formatConnectionUri reads; the rest of ConnectionInput is irrelevant here.
function fmt(over: Partial<FormatInput>): string {
  return formatConnectionUri({
    kind: 'postgres',
    host: 'h',
    port: 5432,
    username: 'u',
    password: null,
    database: null,
    options: {},
    ...over,
  } as FormatInput);
}

describe('formatConnectionUri / parseConnectionUri database round trip (F3)', () => {
  test.each(['my db', 'a#b', 'a?b', 'café'])('postgres database %j survives', (database) => {
    expect(mustParse(fmt({ database })).database).toBe(database);
  });

  test('sqlite path with a space survives', () => {
    const database = '/Users/me/Application Support/x.db';
    const uri = fmt({ kind: 'sqlite', host: null, port: null, database });
    expect(mustParse(uri).database).toBe(database);
  });

  test('malformed escape in the path is unparseable', () => {
    expect(parseConnectionUri('postgresql://h/a%ZZ')).toBeNull();
  });

  test('number and boolean options are kept', () => {
    const uri = fmt({ database: 'd', options: { tls: true, t: 5 } });
    expect(mustParse(uri).params).toEqual({ tls: 'true', t: '5' });
  });
});
