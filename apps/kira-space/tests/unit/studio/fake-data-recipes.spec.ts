import { describe, expect, test } from 'bun:test';
import type { ObjectMeta } from '@shared/domain/tree';
import type { ColumnDescriptor, TypeClass } from '@shared/protocol/page';
import { planWarnings, recipeFor } from '../../../frontend/src/studio/views/grid/fakeData/recipes';
import { parseTypeBounds } from '../../../frontend/src/studio/views/grid/fakeData/typeBounds';
import type { Recipe } from '../../../frontend/src/studio/views/grid/fakeData/types';

// P15 D12: guards the two subjects CLAUDE.md's unit-test bar actually names — recipeFor's
// seven-rule decision table (a "decision structure large enough that no one can hold it in their
// head") and parseTypeBounds's small per-dialect lexer (real boundary cases, not restated logic).

function column(overrides: Partial<ColumnDescriptor> & { name: string }): ColumnDescriptor {
  return {
    dataType: 'text',
    typeClass: 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
    ...overrides,
  };
}

function meta(overrides: Partial<ObjectMeta> = {}): ObjectMeta {
  return {
    path: 'schema:app/table:t',
    kind: 'table',
    name: 't',
    qualifiedName: 'app.t',
    columns: [],
    primaryKey: null,
    foreignKeys: [],
    referencedBy: [],
    indexes: [],
    rowEstimate: null,
    comment: null,
    ...overrides,
  };
}

describe('recipeFor (P15 D4) — the per-column recipe decision table', () => {
  test('rule 1: a generated column is always skipped', () => {
    const c = column({ name: 'computed_total', generated: true, typeClass: 'number' });
    expect(recipeFor(c, null).recipe).toEqual({ kind: 'skip' });
  });

  test('rule 2: a serial/identity PK (a real defaultExpr) is skipped, not sequenced', () => {
    const c = column({ name: 'id', typeClass: 'number', isPrimaryKey: true, nullable: false });
    const m = meta({
      columns: [
        {
          name: 'id',
          position: 1,
          dataType: 'integer',
          nullable: false,
          defaultExpr: "nextval('t_id_seq'::regclass)",
          isPrimaryKey: true,
          comment: null,
        },
      ],
    });
    expect(recipeFor(c, m).recipe).toEqual({ kind: 'skip' });
  });

  test('rule 3: a no-default integer PK gets sequence starting at 1', () => {
    const c = column({ name: 'id', typeClass: 'number', isPrimaryKey: true, nullable: false });
    expect(recipeFor(c, null).recipe).toEqual({ kind: 'sequence', start: 1 });
  });

  test('rule 4: a no-default text PK gets string.uuid', () => {
    const c = column({ name: 'id', typeClass: 'text', isPrimaryKey: true, nullable: false });
    expect(recipeFor(c, null).recipe).toEqual({ kind: 'faker', generatorId: 'string.uuid' });
  });

  test('rule 5: a nullable FK column is skipped; a NOT-NULL one is skipped and warned about', () => {
    const m = meta({
      foreignKeys: [
        {
          name: 'fk_customer',
          columns: ['customer_id'],
          referencedPath: 'schema:app/table:customers',
          referencedColumns: ['id'],
          onDelete: null,
          onUpdate: null,
        },
      ],
    });
    const nullableFk = column({ name: 'customer_id', typeClass: 'number', nullable: true });
    expect(recipeFor(nullableFk, m).recipe).toEqual({ kind: 'skip' });

    const requiredFk = column({ name: 'customer_id', typeClass: 'number', nullable: false });
    const plan = recipeFor(requiredFk, m);
    expect(plan.recipe).toEqual({ kind: 'skip' });
    const warnings = planWarnings([plan], m);
    expect(warnings.some((w) => w.includes('customer_id') && w.includes('foreign key'))).toBe(true);
  });

  test('rule 6: name heuristics hit email/created_at/price when the type agrees', () => {
    expect(recipeFor(column({ name: 'email', typeClass: 'text' }), null).recipe).toEqual({
      kind: 'faker',
      generatorId: 'internet.email',
    });
    expect(recipeFor(column({ name: 'created_at', typeClass: 'temporal' }), null).recipe).toEqual({
      kind: 'faker',
      generatorId: 'date.recent',
    });
    expect(
      recipeFor(column({ name: 'price', typeClass: 'number', dataType: 'numeric(10,2)' }), null)
        .recipe,
    ).toEqual({ kind: 'faker', generatorId: 'finance.amount' });
  });

  test('rule 6 does not fire when the name implies a type the column does not have', () => {
    // 'price' names a number-shaped heuristic (finance.amount, typeClasses ['text','number']) —
    // a 'boolean' column of that name must fall through to its own typeClass fallback instead.
    const plan = recipeFor(column({ name: 'price', typeClass: 'boolean' }), null);
    expect(plan.recipe).toEqual({ kind: 'faker', generatorId: 'datatype.boolean' });
  });

  test('a varchar column named "id" does not get sequence (only a real PK does, rule 3)', () => {
    const c = column({
      name: 'id',
      typeClass: 'text',
      dataType: 'varchar(50)',
      isPrimaryKey: false,
    });
    expect(recipeFor(c, null).recipe).toEqual({ kind: 'faker', generatorId: 'lorem.words' });
  });

  test('rule 7: every typeClass has a fallback, and "other" skips', () => {
    const fallbacks: Record<TypeClass, Recipe> = {
      number: { kind: 'faker', generatorId: 'number.int' },
      text: { kind: 'faker', generatorId: 'lorem.words' },
      boolean: { kind: 'faker', generatorId: 'datatype.boolean' },
      temporal: { kind: 'faker', generatorId: 'date.past' },
      binary: { kind: 'faker', generatorId: 'binary.hex' },
      json: { kind: 'faker', generatorId: 'json.object' },
      other: { kind: 'skip' },
    };
    for (const [typeClass, expected] of Object.entries(fallbacks)) {
      const c = column({ name: 'zzz_unmatched', typeClass: typeClass as TypeClass });
      expect(recipeFor(c, null).recipe).toEqual(expected);
    }
  });
});

describe('planWarnings (P15 D9) — pre-flight warnings recomputed from the current plan', () => {
  test('a unique-index column is named even when its recipe is unique-by-construction', () => {
    // F11: a fresh sequence starting at 1 can still collide with rows already in the table — the
    // client has no way to know what is already stored, regardless of which recipe is chosen. A
    // regression here (warning silenced because the recipe "looks safe") is the one this guards.
    const c = column({
      name: 'tenant_id',
      typeClass: 'number',
      isPrimaryKey: true,
      nullable: false,
    });
    const m = meta({
      indexes: [
        { name: 'pk', columns: ['tenant_id'], unique: true, primary: true, method: 'btree' },
      ],
    });
    const plan = recipeFor(c, m);
    expect(plan.recipe).toEqual({ kind: 'sequence', start: 1 });
    expect(planWarnings([plan], m).some((w) => w.includes('tenant_id'))).toBe(true);
  });

  test('an intentional skip (generated column, defaulted PK) is not warned about', () => {
    const generated = column({ name: 'g', typeClass: 'text', generated: true, nullable: false });
    const plan = { column: generated, recipe: { kind: 'skip' as const } };
    expect(planWarnings([plan], null)).toEqual([]);
  });

  test('a user-set skip on an ordinary NOT-NULL column is warned about', () => {
    const c = column({ name: 'required_note', typeClass: 'text', nullable: false });
    const plan = { column: c, recipe: { kind: 'skip' as const } };
    expect(planWarnings([plan], null).some((w) => w.includes('required_note'))).toBe(true);
  });
});

describe('parseTypeBounds (P15 D5) — the per-dialect declared-type lexer', () => {
  test('varchar(50) and character varying(50) both yield maxLength 50', () => {
    expect(parseTypeBounds('varchar(50)')).toEqual({ maxLength: 50 });
    expect(parseTypeBounds('character varying(50)')).toEqual({ maxLength: 50 });
  });

  test('numeric(20,6) yields precision/scale; bare numeric yields empty bounds', () => {
    expect(parseTypeBounds('numeric(20,6)')).toEqual({ precision: 20, scale: 6 });
    expect(parseTypeBounds('numeric')).toEqual({});
  });

  test('int unsigned clears the signed range down to zero-based', () => {
    expect(parseTypeBounds('int unsigned')).toEqual({
      intRange: { min: 0n, max: 4294967295n },
      signed: false,
    });
  });

  // MariaDB still emits a display width for *signed* integers too (int(11),
  // bigint(20), smallint(6), tinyint(4)) — the old regex only stripped it on the unsigned branch,
  // so these fell through to {} and generation used the unbounded 0..1,000,000 default against a
  // column whose real range could be as narrow as -128..127.
  test('a signed MariaDB display width is stripped just like the unsigned one', () => {
    expect(parseTypeBounds('smallint(6)')).toEqual({
      intRange: { min: -32768n, max: 32767n },
      signed: true,
    });
    expect(parseTypeBounds('tinyint(4)')).toEqual({
      intRange: { min: -128n, max: 127n },
      signed: true,
    });
    expect(parseTypeBounds('int(11)')).toEqual({
      intRange: { min: -2147483648n, max: 2147483647n },
      signed: true,
    });
  });

  test('a signed display width and unsigned together still clear the range to zero-based', () => {
    expect(parseTypeBounds('int(10) unsigned')).toEqual({
      intRange: { min: 0n, max: 4294967295n },
      signed: false,
    });
  });

  test('bigint is a signed 64-bit BigInt range, never a JS number', () => {
    const bounds = parseTypeBounds('bigint');
    expect(bounds.intRange).toEqual({
      min: -9223372036854775808n,
      max: 9223372036854775807n,
    });
  });

  test('Nullable(FixedString(16)) unwraps before matching the length pattern', () => {
    expect(parseTypeBounds('Nullable(FixedString(16))')).toEqual({ maxLength: 16 });
  });

  test("Enum8('a' = 1, 'b,c' = 2) parses both members, including the quoted comma", () => {
    expect(parseTypeBounds("Enum8('a' = 1, 'b,c' = 2)")).toEqual({ enumMembers: ['a', 'b,c'] });
  });

  test('an unrecognised type returns empty bounds rather than throwing', () => {
    expect(parseTypeBounds('geometry(point,4326)')).toEqual({});
  });
});
