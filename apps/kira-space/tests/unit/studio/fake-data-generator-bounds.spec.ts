// P21 round 3 functional findings 1 and 2: fake-data generation ignored the column's declared
// length/precision for every generator except lorem.* (finding 1) and offered finance.amount
// (price/amount/total/cost/salary) for integer and narrow-numeric columns with no reference to
// their real bounds (finding 2) — both failure modes abort the whole batch on the first row that
// overflows, with committedRows = 0 and nothing in the plan warning it could happen.
// generate.ts imports bridge/data.ts, which reaches '/wails/runtime.js' at module scope — this has
// to be a dynamic import(), after @workbench/testing/unit/window's mock.module registration has run (the same
// pattern fake-data-temporal-format.spec.ts already uses).
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ColumnPlan, GeneratorId } from '../../../frontend/src/studio/views/grid/fakeData/types';

const { previewFirstRows } = await import('../../../frontend/src/studio/views/grid/fakeData/generate');

function column(name: string, dataType: string): ColumnDescriptor {
  return {
    name,
    dataType,
    typeClass: 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  };
}

function planFor(name: string, dataType: string, generatorId: GeneratorId): ColumnPlan {
  return { column: column(name, dataType), recipe: { kind: 'faker', generatorId } };
}

async function valuesFor(plan: ColumnPlan, count = 200): Promise<(string | null)[]> {
  const rows = await previewFirstRows([plan], 1, count, 'postgres');
  return rows.map((r) => (r.kind === 'insert' ? r.values[plan.column.name] : null));
}

describe('fake-data generation respects the column bounds it already parses out (finding 1)', () => {
  const generators: [GeneratorId, string][] = [
    ['person.fullName', 'name'],
    ['person.firstName', 'first_name'],
    ['person.lastName', 'last_name'],
    ['internet.email', 'email'],
    ['internet.url', 'url'],
    ['phone.number', 'phone'],
    ['location.city', 'city'],
    ['location.country', 'country'],
    ['location.state', 'state'],
    ['location.zipCode', 'zip'],
    ['location.streetAddress', 'address'],
    ['company.name', 'company'],
  ];

  for (const [generatorId, columnName] of generators) {
    test(`${generatorId} into varchar(5) never exceeds 5 characters`, async () => {
      const values = await valuesFor(planFor(columnName, 'varchar(5)', generatorId));
      for (const v of values) {
        expect(typeof v).toBe('string');
        expect((v as string).length).toBeLessThanOrEqual(5);
      }
    });
  }

  test('string.uuid into char(8) is clamped rather than left at its full 36 characters', async () => {
    const values = await valuesFor(planFor('id', 'char(8)', 'string.uuid'));
    for (const v of values) {
      expect((v as string).length).toBeLessThanOrEqual(8);
    }
  });

  test('json.object into varchar(10) is clamped', async () => {
    const values = await valuesFor(planFor('meta', 'varchar(10)', 'json.object'));
    for (const v of values) {
      expect((v as string).length).toBeLessThanOrEqual(10);
    }
  });

  test('binary.hex derives its length from a FixedString(n) bound instead of a fixed 16 hex chars', async () => {
    const values = await valuesFor(planFor('payload', 'FixedString(4)', 'binary.hex'));
    for (const v of values) {
      // "0x" + 2 hex chars per byte.
      expect(v).toMatch(/^0x[0-9a-f]{8}$/);
    }
  });

  test('binary.hex keeps its previous 16-hex-char default when no bound is known', async () => {
    const values = await valuesFor(planFor('payload', 'blob', 'binary.hex'));
    for (const v of values) {
      expect(v).toMatch(/^0x[0-9a-f]{16}$/);
    }
  });
});

describe('finance.amount respects the column’s real numeric bounds (finding 2)', () => {
  test('an integer column gets a plain integer, never a decimal string faker.finance.amount would produce', async () => {
    const values = await valuesFor(planFor('price', 'integer', 'finance.amount'));
    for (const v of values) {
      expect(v).toMatch(/^-?\d+$/);
      expect(Number(v)).toBeGreaterThanOrEqual(-2147483648);
      expect(Number(v)).toBeLessThanOrEqual(2147483647);
    }
  });

  test('a narrow numeric(4,2) column never overflows its 2-digit whole part', async () => {
    const values = await valuesFor(planFor('total', 'numeric(4,2)', 'finance.amount'));
    for (const v of values) {
      expect(v).toMatch(/^\d{1,2}\.\d{2}$/);
      expect(Number(v)).toBeLessThanOrEqual(99.99);
    }
  });

  test('numeric(2,2) (no whole-number digits) never overflows, was 9.99 before F19 (P108 Part 10)', async () => {
    const values = await valuesFor(planFor('rate', 'numeric(2,2)', 'finance.amount'));
    for (const v of values) {
      expect(v).toMatch(/^0\.\d{2}$/);
      expect(Number(v)).toBeLessThan(1);
    }
  });

  test('a plain text column (no numeric bounds) keeps the previous 2-decimal default behaviour', async () => {
    const values = await valuesFor(planFor('cost', 'varchar(255)', 'finance.amount'));
    for (const v of values) {
      expect(v).toMatch(/^\d+\.\d{2}$/);
    }
  });
});
