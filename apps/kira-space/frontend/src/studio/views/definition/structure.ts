import type { ConstraintMeta, ObjectDefinition } from '@shared/domain/definition';
import { type ForeignKeyMeta, type ObjectMeta, pathTail } from '@shared/domain/tree';

/** One Constraints-section row, merged from ObjectDefinition.constraints (engine text) and the
 *  ObjectMeta edges the SQL adapters already return (foreignKeys/referencedBy), de-duplicated by
 *  name so a foreign key never renders twice (P19 D11). */
export interface ConstraintRow {
  name: string;
  type: ConstraintMeta['type'] | 'referencedBy';
  detail: string;
  /** Set for 'foreignKey'/'referencedBy' — the encoded path of the other table (P7's own field). */
  referencedPath?: string;
}

function foreignKeyDetail(fk: ForeignKeyMeta): string {
  return `(${fk.columns.join(', ')}) REFERENCES (${fk.referencedColumns.join(', ')})`;
}

function referencedByDetail(fk: ForeignKeyMeta): string {
  const otherTable = pathTail(fk.referencedPath)?.name ?? fk.referencedPath;
  return `${otherTable} (${fk.referencedColumns.join(', ')}) -> (${fk.columns.join(', ')})`;
}

export function buildConstraintRows(
  definition: ObjectDefinition,
  meta: ObjectMeta,
): ConstraintRow[] {
  const rows: ConstraintRow[] = definition.constraints.map((c) => {
    const row: ConstraintRow = { name: c.name, type: c.type, detail: c.definition };
    if (c.type === 'foreignKey') {
      const fk = meta.foreignKeys.find((f) => f.name === c.name);
      if (fk) row.referencedPath = fk.referencedPath;
    }
    return row;
  });

  // Defensive merge: every outbound FK is expected to already have a matching named constraint
  // above (both adapters build `constraints` from the same catalog rows that back
  // ObjectMeta.foreignKeys), but a mismatch must not silently drop the edge from the section.
  const seen = new Set(rows.map((r) => r.name));
  for (const fk of meta.foreignKeys) {
    if (seen.has(fk.name)) continue;
    seen.add(fk.name);
    rows.push({
      name: fk.name,
      type: 'foreignKey',
      detail: foreignKeyDetail(fk),
      referencedPath: fk.referencedPath,
    });
  }

  // Inbound references live on the *other* table's own constraint list, never on this table's
  // `definition.constraints` — always their own rows, never merged against the loop above.
  for (const fk of meta.referencedBy) {
    rows.push({
      name: fk.name,
      type: 'referencedBy',
      detail: referencedByDetail(fk),
      referencedPath: fk.referencedPath,
    });
  }

  return rows;
}

export interface JsonSchemaFieldRow {
  name: string;
  bsonType: string | null;
  required: boolean;
  description: string | null;
}

/** Parses a Mongo `$jsonSchema` sub-document's `properties`/`required` into field rows for the
 *  Validation section's table. Returns null when `validator` doesn't parse as an object with a
 *  `properties` map — the signal to render it as raw JSON instead of a table (P19 D12). */
export function jsonSchemaFields(validator: string): JsonSchemaFieldRow[] | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(validator);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null) return null;
  const obj = parsed as Record<string, unknown>;
  if (typeof obj.properties !== 'object' || obj.properties === null) return null;

  const required = new Set(
    Array.isArray(obj.required)
      ? obj.required.filter((r): r is string => typeof r === 'string')
      : [],
  );

  const rows: JsonSchemaFieldRow[] = [];
  for (const [name, rawFieldSchema] of Object.entries(obj.properties as Record<string, unknown>)) {
    if (typeof rawFieldSchema !== 'object' || rawFieldSchema === null) continue;
    const fieldSchema = rawFieldSchema as Record<string, unknown>;
    const rawType = fieldSchema.bsonType ?? fieldSchema.type;
    const bsonType = Array.isArray(rawType)
      ? rawType.filter((t): t is string => typeof t === 'string').join(' | ')
      : typeof rawType === 'string'
        ? rawType
        : null;
    rows.push({
      name,
      bsonType,
      required: required.has(name),
      description: typeof fieldSchema.description === 'string' ? fieldSchema.description : null,
    });
  }
  return rows;
}
