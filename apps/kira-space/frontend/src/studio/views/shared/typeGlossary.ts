import { type MessageKey, t } from '@theme/i18n';

// A short human-readable description for a column/field data type. Covers every type the app
// can show: the numeric family, the char/varchar/text/blob families, the temporal family, boolean,
// serial/identity, arrays, plus MariaDB's own spellings and the BSON type names $jsonSchema uses
// for Mongo's Validation section — the earlier "obvious types need no gloss" rationale didn't
// survive contact with catalog spellings like int2/int8/float8/bpchar/timestamptz, which are not
// obvious at all. Genuinely self-explanatory container/primitive names (string, array, object,
// null) are still left uncovered on purpose — the definition view's Columns section only shows an
// info icon when there is something to explain.
const DESCRIPTIONS: readonly { test: RegExp; key: MessageKey }[] = [
  // --- temporal family -------------------------------------------------------------------
  {
    test: /^tsrange$/,
    key: 'studio.typeGlossary.tsrange.description',
  },
  { test: /^tstzrange$/, key: 'studio.typeGlossary.tstzrange.description' },
  { test: /^daterange$/, key: 'studio.typeGlossary.daterange.description' },
  { test: /^interval$/, key: 'studio.typeGlossary.interval.description' },
  { test: /^date$/, key: 'studio.typeGlossary.date.description' },
  { test: /^time(\s+without\s+time\s+zone)?$/, key: 'studio.typeGlossary.time.description' },
  {
    test: /^(timetz|time\s+with\s+time\s+zone)$/,
    key: 'studio.typeGlossary.timetz.description',
  },
  {
    // "timestamp" alone means without-time-zone in both Postgres and MariaDB's own DATETIME-like
    // reading here — distinguished from timestamptz below, per the ask's own explicit callout.
    test: /^(timestamp(\s+without\s+time\s+zone)?|datetime)$/,
    key: 'studio.typeGlossary.timestamp.description',
  },
  {
    test: /^(timestamptz|timestamp\s+with\s+time\s+zone)$/,
    key: 'studio.typeGlossary.timestamptz.description',
  },
  { test: /^year$/, key: 'studio.typeGlossary.year.description' },

  // --- numeric family ----------------------------------------------------------------------
  { test: /^int4range$/, key: 'studio.typeGlossary.int4range.description' },
  { test: /^int8range$/, key: 'studio.typeGlossary.int8range.description' },
  { test: /^numrange$/, key: 'studio.typeGlossary.numrange.description' },
  {
    test: /^(tinyint)(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.tinyint.description',
  },
  {
    test: /^(smallint|int2)(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.smallint.description',
  },
  {
    test: /^mediumint(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.mediumint.description',
  },
  {
    test: /^(integer|int|int4)(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.integer.description',
  },
  {
    // "long" is BSON's own name for a 64-bit integer ($jsonSchema's bsonType) — grouped with
    // bigint/int8 rather than with int/integer above, unlike its similar spelling.
    test: /^(bigint|int8|long)(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.bigint.description',
  },
  {
    test: /^(decimal|numeric)(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.decimal.description',
  },
  {
    test: /^(real|float4|float)(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.float32.description',
  },
  {
    test: /^(double(\s+precision)?|float8)(\s+unsigned)?(\s+zerofill)?$/,
    key: 'studio.typeGlossary.float64.description',
  },
  { test: /^money$/, key: 'studio.typeGlossary.money.description' },

  // --- serial / identity ---------------------------------------------------------------------
  {
    test: /^(smallserial|serial2)$/,
    key: 'studio.typeGlossary.smallserial.description',
  },
  {
    test: /^(serial|serial4)$/,
    key: 'studio.typeGlossary.serial.description',
  },
  {
    test: /^(bigserial|serial8)$/,
    key: 'studio.typeGlossary.bigserial.description',
  },

  // --- boolean -------------------------------------------------------------------------------
  { test: /^bool(ean)?$/, key: 'studio.typeGlossary.bool.description' },

  // --- char/varchar/text/blob families ---------------------------------------------------------
  {
    test: /^(character|char|bpchar)$/,
    key: 'studio.typeGlossary.char.description',
  },
  {
    test: /^(character\s+varying|varchar)$/,
    key: 'studio.typeGlossary.varchar.description',
  },
  { test: /^text$/, key: 'studio.typeGlossary.text.description' },
  { test: /^tinytext$/, key: 'studio.typeGlossary.tinytext.description' },
  { test: /^mediumtext$/, key: 'studio.typeGlossary.mediumtext.description' },
  { test: /^longtext$/, key: 'studio.typeGlossary.longtext.description' },
  { test: /^bytea$/, key: 'studio.typeGlossary.bytea.description' },
  { test: /^binary$/, key: 'studio.typeGlossary.binary.description' },
  {
    test: /^varbinary$/,
    key: 'studio.typeGlossary.varbinary.description',
  },
  { test: /^tinyblob$/, key: 'studio.typeGlossary.tinyblob.description' },
  {
    // P35 D31: "blob" alone is no longer a MariaDB-only spelling — it's also SQLite's own native
    // blob type (F21), which has no such size cap, so a single hard number would be wrong for one
    // of the two engines this entry now has to describe.
    test: /^blob$/,
    key: 'studio.typeGlossary.blob.description',
  },
  { test: /^mediumblob$/, key: 'studio.typeGlossary.mediumblob.description' },
  { test: /^longblob$/, key: 'studio.typeGlossary.longblob.description' },

  // --- identifiers, structured/exotic types ---------------------------------------------------
  { test: /^uuid$/, key: 'studio.typeGlossary.uuid.description' },
  {
    test: /^jsonb?$/,
    key: 'studio.typeGlossary.json.description',
  },
  { test: /^inet$/, key: 'studio.typeGlossary.inet.description' },
  {
    test: /^cidr$/,
    key: 'studio.typeGlossary.cidr.description',
  },
  { test: /^macaddr8?$/, key: 'studio.typeGlossary.macaddr.description' },
  { test: /^xml$/, key: 'studio.typeGlossary.xml.description' },
  {
    test: /^(point|line|lseg|box|path|polygon|circle)$/,
    key: 'studio.typeGlossary.geometric.description',
  },
  { test: /^(var)?bit$/, key: 'studio.typeGlossary.bit.description' },
  { test: /^tsvector$/, key: 'studio.typeGlossary.tsvector.description' },
  { test: /^tsquery$/, key: 'studio.typeGlossary.tsquery.description' },
  {
    test: /^oid$/,
    key: 'studio.typeGlossary.oid.description',
  },
  { test: /^enum$/, key: 'studio.typeGlossary.enum.description' },
  {
    // Was `/^set\(/` before D28 introduced normalize()'s qualifier-stripping below — the
    // "('a','b')" list is a qualifier like any other now, so this matches the same way
    // varchar/numeric's own qualifiers do rather than relying on the paren surviving.
    test: /^set$/,
    key: 'studio.typeGlossary.set.description',
  },
  { test: /^geometry|geography/, key: 'studio.typeGlossary.spatial.description' },
  {
    // P35 D21/D31: SQLite's own STRICT-table type — the one declared type with no affinity rule
    // at all (F21), so a value here can genuinely be any SQLite storage class.
    test: /^any$/,
    key: 'studio.typeGlossary.any.description',
  },

  // --- Mongo BSON type names (a $jsonSchema validator's own "bsonType" spellings) -------------
  // Genuinely self-explanatory ones (string, array, object, null) are left uncovered, same
  // reasoning as the SQL side. "int"/"long"/"bool"/"decimal" reuse the SQL entries above — the
  // meaning (32-bit int, 64-bit int, true/false, exact arbitrary-precision) is identical.
  { test: /^objectid$/, key: 'studio.typeGlossary.objectid.description' },
  { test: /^bindata$/, key: 'studio.typeGlossary.bytea.description' },
  {
    test: /^javascript$/,
    key: 'studio.typeGlossary.script.description',
  },
  { test: /^regex$/, key: 'studio.typeGlossary.regex.description' },
  {
    test: /^minkey$/,
    key: 'studio.typeGlossary.minkey.description',
  },
  {
    test: /^maxkey$/,
    key: 'studio.typeGlossary.maxkey.description',
  },
];

// --- ClickHouse's own type family -------------------------------------------------------------
// Matched case-sensitively against the *un-normalized* text, before normalize()'s
// lowercasing — ClickHouse's own catalog always spells these in exact PascalCase, which is
// what lets "Int8" (an 8-bit signed integer here) resolve to its own entry instead of colliding
// with the generic DESCRIPTIONS list's lowercase "int8" (Postgres/MariaDB's own shorthand for a
// 64-bit bigint, a completely different width) — the two vocabularies use the same six characters
// for unrelated meanings, and only case tells them apart.
const CLICKHOUSE_DESCRIPTIONS: readonly { test: RegExp; key: MessageKey }[] = [
  { test: /^UInt8$/, key: 'studio.typeGlossary.uint8.description' },
  { test: /^UInt16$/, key: 'studio.typeGlossary.uint16.description' },
  { test: /^UInt32$/, key: 'studio.typeGlossary.uint32.description' },
  { test: /^UInt64$/, key: 'studio.typeGlossary.uint64.description' },
  { test: /^UInt(128|256)$/, key: 'studio.typeGlossary.uint128.description' },
  { test: /^Int8$/, key: 'studio.typeGlossary.int8.description' },
  { test: /^Int16$/, key: 'studio.typeGlossary.int16.description' },
  { test: /^Int32$/, key: 'studio.typeGlossary.int32.description' },
  { test: /^Int64$/, key: 'studio.typeGlossary.int64.description' },
  { test: /^Int(128|256)$/, key: 'studio.typeGlossary.int128.description' },
  { test: /^Float32$/, key: 'studio.typeGlossary.float32.description' },
  { test: /^Float64$/, key: 'studio.typeGlossary.float64.description' },
  {
    test: /^Decimal(32|64|128|256)?$/,
    key: 'studio.typeGlossary.fixedDecimal.description',
  },
  { test: /^Bool$/, key: 'studio.typeGlossary.boolUnsigned.description' },
  { test: /^String$/, key: 'studio.typeGlossary.string.description' },
  {
    test: /^FixedString$/,
    key: 'studio.typeGlossary.fixedString.description',
  },
  { test: /^Date$/, key: 'studio.typeGlossary.dateFrom1970.description' },
  { test: /^Date32$/, key: 'studio.typeGlossary.date32.description' },
  {
    test: /^DateTime$/,
    key: 'studio.typeGlossary.dateTime.description',
  },
  {
    test: /^DateTime64$/,
    key: 'studio.typeGlossary.dateTime64.description',
  },
  { test: /^Time$/, key: 'studio.typeGlossary.timeNoDate.description' },
  { test: /^Time64$/, key: 'studio.typeGlossary.time64.description' },
  { test: /^UUID$/, key: 'studio.typeGlossary.uuid.description' },
  { test: /^IPv4$/, key: 'studio.typeGlossary.ipv4.description' },
  { test: /^IPv6$/, key: 'studio.typeGlossary.ipv6.description' },
  {
    test: /^Enum(8|16)?$/,
    key: 'studio.typeGlossary.enumInt.description',
  },
  { test: /^Array$/, key: 'studio.typeGlossary.array.description' },
  { test: /^Tuple$/, key: 'studio.typeGlossary.tuple.description' },
  { test: /^Map$/, key: 'studio.typeGlossary.map.description' },
  { test: /^Nested$/, key: 'studio.typeGlossary.nested.description' },
  {
    test: /^(Point|Ring|Polygon|MultiPolygon)$/,
    key: 'studio.typeGlossary.geoShape.description',
  },
  { test: /^JSON$/, key: 'studio.typeGlossary.jsonDynamic.description' },
  {
    test: /^(Aggregate|SimpleAggregate)Function$/,
    key: 'studio.typeGlossary.aggState.description',
  },
];

// Strips Nullable(...) and LowCardinality(...) wrappers, recursively, in either nesting
// order — the same two wrappers read.ts's own unwrapType strips server-side. Without this,
// normalize()'s single, non-nested paren-stripping regex mangles "Nullable(LowCardinality(String))"
// into "nullable)" (it stops at the *first* closing paren, the inner one) instead of reaching the
// wrapped type at all.
function unwrapClickHouseWrappers(dataType: string): string {
  let inner = dataType.trim();
  for (;;) {
    const nullableMatch = /^Nullable\((.*)\)$/s.exec(inner);
    if (nullableMatch?.[1] !== undefined) {
      inner = nullableMatch[1].trim();
      continue;
    }
    const lowCardMatch = /^LowCardinality\((.*)\)$/s.exec(inner);
    if (lowCardMatch?.[1] !== undefined) {
      inner = lowCardMatch[1].trim();
      continue;
    }
    break;
  }
  return inner;
}

// Strips a length/precision qualifier — "varchar(64)" -> "varchar", "numeric(10,2)" -> "numeric"
// — so the lookup above matches on the base type name alone (D28).
function normalize(dataType: string): string {
  return dataType
    .toLowerCase()
    .replace(/\([^)]*\)/, '')
    .replace(/\s+/g, ' ')
    .trim();
}

export function typeDescription(dataType: string): string | null {
  const unwrapped = unwrapClickHouseWrappers(dataType);
  const chBase = unwrapped.replace(/\(.*$/s, '').trim();
  const chMatch = CLICKHOUSE_DESCRIPTIONS.find((d) => d.test.test(chBase))?.key;
  if (chMatch) return t(chMatch);

  const type = normalize(unwrapped);
  // Arrays (Postgres's "integer[]", "text[]", …): describe by the element type rather than
  // listing every possible array shape as its own entry.
  const arrayMatch = type.match(/^(.+?)((?:\[\])+)$/);
  if (arrayMatch?.[1]) {
    const elementDescription = typeDescription(arrayMatch[1]);
    return elementDescription
      ? t('studio.typeGlossary.arrayOfWith.description', {
          element: arrayMatch[1],
          description: elementDescription.replace(/\.$/, ''),
        })
      : t('studio.typeGlossary.arrayOf.description', { element: arrayMatch[1] });
  }
  const key = DESCRIPTIONS.find((d) => d.test.test(type))?.key;
  return key ? t(key) : null;
}
