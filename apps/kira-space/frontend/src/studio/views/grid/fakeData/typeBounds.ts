// The declared type's own length/precision/enum-member bounds, parsed out of `dataType` — the
// server's verbatim, per-dialect type string (`ColumnDescriptor.dataType`, e.g.
// 'varchar(50)', 'numeric(20,6)', 'int unsigned', 'Enum8(\'a\' = 1, \'b\' = 2)'). `typeClass`
// alone collapses all of these onto seven buckets and can't answer "how long" or "which members".
//
// Integer ranges are BigInt, never `number` — a `bigint`/`UInt64` column's range exceeds
// Number.MAX_SAFE_INTEGER, and every generated value travels as text (RowValues' `*string`), so
// nothing here ever needs to fit a JS float64 ("a bigint is generated as a decimal string,
// never a JavaScript number").
export interface TypeBounds {
  maxLength?: number;
  precision?: number;
  scale?: number;
  signed?: boolean;
  enumMembers?: string[];
  intRange?: { min: bigint; max: bigint };
}

// ClickHouse's own fixed-width integer types are case-sensitive and collide by name with
// Postgres/MySQL's lowercase ones if naively lowercased first ('Int8' is an 8-bit signed integer;
// postgres's 'int8' — never emitted by format_type, but listed defensively below — is a
// 64-bit bigint). Matched case-sensitively, before anything is lowercased, so the two families
// never conflate.
const CLICKHOUSE_INT_RANGES: Record<string, { min: bigint; max: bigint }> = {
  Int8: { min: -128n, max: 127n },
  UInt8: { min: 0n, max: 255n },
  Int16: { min: -32768n, max: 32767n },
  UInt16: { min: 0n, max: 65535n },
  Int32: { min: -2147483648n, max: 2147483647n },
  UInt32: { min: 0n, max: 4294967295n },
  Int64: { min: -9223372036854775808n, max: 9223372036854775807n },
  UInt64: { min: 0n, max: 18446744073709551615n },
};

const SQL_INT_RANGES: Record<string, { min: bigint; max: bigint; unsignedMax: bigint }> = {
  tinyint: { min: -128n, max: 127n, unsignedMax: 255n },
  smallint: { min: -32768n, max: 32767n, unsignedMax: 65535n },
  int2: { min: -32768n, max: 32767n, unsignedMax: 65535n },
  mediumint: { min: -8388608n, max: 8388607n, unsignedMax: 16777215n },
  int: { min: -2147483648n, max: 2147483647n, unsignedMax: 4294967295n },
  integer: { min: -2147483648n, max: 2147483647n, unsignedMax: 4294967295n },
  int4: { min: -2147483648n, max: 2147483647n, unsignedMax: 4294967295n },
  bigint: {
    min: -9223372036854775808n,
    max: 9223372036854775807n,
    unsignedMax: 18446744073709551615n,
  },
  int8: {
    min: -9223372036854775808n,
    max: 9223372036854775807n,
    unsignedMax: 18446744073709551615n,
  },
};

function unwrapNullable(dataType: string): string {
  const match = dataType.match(/^Nullable\((.+)\)$/s);
  return match ? match[1] : dataType;
}

// Scans `'a' = 1, 'b,c' = 2`'s member list by quote state rather than splitting on ',' — a comma
// inside a quoted member name (D12's own boundary case) must not end that member early. `\'` is
// ClickHouse's own escape for a literal quote inside the string.
function parseEnumMembers(inner: string): string[] {
  const members: string[] = [];
  let i = 0;
  while (i < inner.length) {
    while (i < inner.length && /[\s,]/.test(inner[i])) i++;
    if (i >= inner.length || inner[i] !== "'") break;
    i++; // opening quote
    let value = '';
    while (i < inner.length) {
      if (inner[i] === '\\' && i + 1 < inner.length) {
        value += inner[i + 1];
        i += 2;
        continue;
      }
      if (inner[i] === "'") {
        i++;
        break;
      }
      value += inner[i];
      i++;
    }
    members.push(value);
    while (i < inner.length && inner[i] !== ',') i++;
  }
  return members;
}

export function parseTypeBounds(dataType: string): TypeBounds {
  const unwrapped = unwrapNullable(dataType.trim());

  const chInt = CLICKHOUSE_INT_RANGES[unwrapped];
  if (chInt) return { intRange: chInt, signed: chInt.min < 0n };

  const enumMatch = unwrapped.match(/^Enum(?:8|16)\((.*)\)$/s);
  if (enumMatch) {
    const members = parseEnumMembers(enumMatch[1]);
    return members.length > 0 ? { enumMembers: members } : {};
  }

  const lengthMatch = unwrapped.match(
    /^(?:national\s+)?(?:character varying|varchar|char|nchar|nvarchar|FixedString)\s*\(\s*(\d+)\s*\)$/i,
  );
  if (lengthMatch) return { maxLength: Number(lengthMatch[1]) };

  const numericMatch = unwrapped.match(/^(?:numeric|decimal)\s*\(\s*(\d+)\s*(?:,\s*(\d+)\s*)?\)$/i);
  if (numericMatch) {
    return {
      precision: Number(numericMatch[1]),
      scale: numericMatch[2] !== undefined ? Number(numericMatch[2]) : 0,
    };
  }

  // mysqlfamily reads COLUMN_TYPE, not DATA_TYPE, so an unsigned int arrives as
  // "int unsigned" (occasionally with a now-vestigial display width, "int(10) unsigned"). The
  // display-width group used to be stripped only on the unsigned branch — MariaDB
  // still emits one for signed integers too (int(11), bigint(20), smallint(6), tinyint(4)), which
  // wasn't a key in SQL_INT_RANGES, so parseTypeBounds silently returned {} and generation fell
  // through to its unbounded 0..1,000,000 default against columns with real, much narrower
  // ranges. Stripped on both branches now via one match, signed or not.
  const intMatch = unwrapped.match(/^(\w+)(?:\s*\(\s*\d+\s*\))?(?:\s+(unsigned))?$/i);
  const base = (intMatch ? intMatch[1] : unwrapped).toLowerCase();
  const unsigned = !!intMatch?.[2];
  const sqlInt = SQL_INT_RANGES[base];
  if (sqlInt) {
    const signed = !unsigned;
    return {
      intRange: signed
        ? { min: sqlInt.min, max: sqlInt.max }
        : { min: 0n, max: sqlInt.unsignedMax },
      signed,
    };
  }

  return {};
}
