// The DDL extractor (ddl.ts) earns a unit test — a tree walk over four dialects' grammars
// with interacting rules: qualified vs. unqualified names, quoted vs. bare vs. keyword-shaped
// column names (the `id`), nested type-argument parens, table constraints that are not
// columns, `IF NOT EXISTS`, and statements it must skip silently.

import { describe, expect, test } from 'bun:test';
import type { RelationColumns } from '@shared/domain/tree';
import {
  defaultSchemaFor,
  namespaceFromCached,
  parseDdl,
  toSqlNamespace,
} from '../../../frontend/src/studio/views/console/ddl';

describe('parseDdl — one table per dialect', () => {
  test('postgres: qualified table, PK/UNIQUE/REFERENCES flags', () => {
    const schema = parseDdl(
      'postgres',
      `CREATE TABLE public.users (
        id integer NOT NULL PRIMARY KEY,
        email varchar(255) UNIQUE,
        org_id integer REFERENCES orgs(id)
      );`,
    );
    expect(schema.tables).toHaveLength(1);
    const table = schema.tables[0];
    if (!table) throw new Error('expected one table');
    expect(table.name).toBe('users');
    expect(table.schema).toBe('public');
    expect(table.columns.map((c) => c.name)).toEqual(['id', 'email', 'org_id']);
    expect(table.columns[0]).toMatchObject({
      name: 'id',
      type: 'integer',
      notNull: true,
      primaryKey: true,
    });
    expect(table.columns[1]).toMatchObject({ name: 'email', type: 'varchar(255)', unique: true });
    expect(table.columns[2]?.references).toEqual({ table: 'orgs', column: 'id' });
  });

  test('mysql: backtick-quoted identifiers and an inline comment', () => {
    const schema = parseDdl(
      'mysql',
      "CREATE TABLE `orders` (\n  `id` INT NOT NULL AUTO_INCREMENT,\n  `total` DECIMAL(10,2) DEFAULT '0.00', -- money\n  PRIMARY KEY (`id`)\n);",
    );
    expect(schema.tables).toHaveLength(1);
    const table = schema.tables[0];
    if (!table) throw new Error('expected one table');
    expect(table.name).toBe('orders');
    expect(table.columns.map((c) => c.name)).toEqual(['id', 'total']);
    // F5.2: verbatim type slice, not a reconstruction — proves numeric(20,6)-shaped types survive.
    expect(table.columns[1]?.type).toBe('DECIMAL(10,2)');
    // The table-level `PRIMARY KEY (\`id\`)` constraint must not become a phantom column.
    expect(table.columns.some((c) => c.name.toUpperCase() === 'PRIMARY')).toBe(false);
  });

  test('sqlite: INTEGER PRIMARY KEY AUTOINCREMENT and NOT NULL/UNIQUE', () => {
    const schema = parseDdl(
      'sqlite',
      'CREATE TABLE users (\n  id INTEGER PRIMARY KEY AUTOINCREMENT,\n  name TEXT NOT NULL,\n  email TEXT UNIQUE\n);',
    );
    const table = schema.tables[0];
    if (!table) throw new Error('expected one table');
    expect(table.columns[0]).toMatchObject({ name: 'id', type: 'INTEGER', primaryKey: true });
    expect(table.columns[1]).toMatchObject({ name: 'name', type: 'TEXT', notNull: true });
    expect(table.columns[2]).toMatchObject({ name: 'email', type: 'TEXT', unique: true });
  });

  test('clickhouse: nested type-argument parens, and DEFAULT tokenised as a plain Identifier', () => {
    const schema = parseDdl(
      'clickhouse',
      'CREATE TABLE default.events (\n  id UUID,\n  tags Array(LowCardinality(String)),\n  created DateTime64(3) DEFAULT now()\n) ENGINE = MergeTree() ORDER BY id;',
    );
    const table = schema.tables[0];
    if (!table) throw new Error('expected one table');
    expect(table.schema).toBe('default');
    expect(table.columns[1]).toMatchObject({ name: 'tags', type: 'Array(LowCardinality(String))' });
    // DEFAULT tokenises as a Keyword under this repo's curated ClickHouse set (sql-keywords.ts's
    // REQUIRED_MINIMUM, needed for TYPE_STOP_WORDS to work at all) — the type slice must still
    // stop there, not swallow "DateTime64(3) DEFAULT now()".
    expect(table.columns[2]).toMatchObject({ name: 'created', type: 'DateTime64(3)' });
  });
});

describe('parseDdl — F5.1: a column literally named `id`/`name` is a Postgres Keyword node', () => {
  test('id and name both parse as columns, not as dropped tokens', () => {
    const schema = parseDdl('postgres', 'CREATE TABLE t (id integer, name text);');
    expect(schema.tables[0]?.columns.map((c) => c.name)).toEqual(['id', 'name']);
  });
});

describe('parseDdl — table-level constraints are consumed, not emitted as columns', () => {
  test('PRIMARY KEY (a, b)', () => {
    const schema = parseDdl(
      'postgres',
      'CREATE TABLE t (\n  a integer,\n  b integer,\n  PRIMARY KEY (a, b)\n);',
    );
    expect(schema.tables[0]?.columns.map((c) => c.name)).toEqual(['a', 'b']);
  });

  test('a named FOREIGN KEY constraint', () => {
    const schema = parseDdl(
      'postgres',
      'CREATE TABLE t (\n  id integer,\n  org_id integer,\n  CONSTRAINT fk_org FOREIGN KEY (org_id) REFERENCES orgs(id)\n);',
    );
    expect(schema.tables[0]?.columns.map((c) => c.name)).toEqual(['id', 'org_id']);
  });
});

describe('parseDdl — IF NOT EXISTS, and CREATE INDEX marking a column indexed', () => {
  test('IF NOT EXISTS is skipped before the table name', () => {
    const schema = parseDdl('postgres', 'CREATE TABLE IF NOT EXISTS public.users (id integer);');
    expect(schema.tables[0]).toMatchObject({ name: 'users', schema: 'public' });
  });

  test('CREATE INDEX marks the indexed column (surfaced on hover, not completion)', () => {
    const schema = parseDdl(
      'postgres',
      'CREATE TABLE users (id integer, email varchar(255));\nCREATE INDEX users_email_idx ON users (email);',
    );
    const table = schema.tables[0];
    if (!table) throw new Error('expected one table');
    expect(table.columns.find((c) => c.name === 'email')?.indexed).toBe(true);
    expect(table.columns.find((c) => c.name === 'id')?.indexed).toBeUndefined();
  });
});

describe('parseDdl — ALTER TABLE ADD COLUMN and COMMENT ON COLUMN', () => {
  test('ALTER TABLE adds a column to an already-declared table', () => {
    const schema = parseDdl(
      'postgres',
      'CREATE TABLE users (id integer);\nALTER TABLE users ADD COLUMN age integer;',
    );
    expect(schema.tables[0]?.columns.map((c) => c.name)).toEqual(['id', 'age']);
  });

  test('COMMENT ON COLUMN sets the description used by the hover', () => {
    // Deliberately no embedded apostrophe: this Lezer grammar tokenises `''` as two adjacent
    // String literals rather than one escaped quote — a real grammar limitation (the
    // "tokeniser, not a structural parser"), not something this extractor
    // can fix, so the description this rule captures is best-effort for that case.
    const schema = parseDdl(
      'postgres',
      "CREATE TABLE users (id integer, email text);\nCOMMENT ON COLUMN public.users.email IS 'the users email';",
    );
    expect(schema.tables[0]?.columns.find((c) => c.name === 'email')?.description).toBe(
      'the users email',
    );
  });
});

describe('parseDdl — statements this extractor cannot make sense of are skipped silently', () => {
  test('a pg_dump preamble (SET/GRANT/a header comment) contributes nothing but does not stop the real table', () => {
    const schema = parseDdl(
      'postgres',
      'SET search_path = public;\nGRANT SELECT ON users TO app;\n-- header comment\nCREATE TABLE t (id integer);',
    );
    expect(schema.tables).toHaveLength(1);
    expect(schema.tables[0]?.name).toBe('t');
  });

  test('an unterminated CREATE TABLE never throws', () => {
    expect(() =>
      parseDdl('postgres', 'CREATE TABLE t (id integer NOT NULL PRIMARY KEY'),
    ).not.toThrow();
  });
});

describe('toSqlNamespace — D10: a qualified table is emitted both nested and flat', () => {
  test('public.users completes as both `public.users` and bare `users`', () => {
    const schema = parseDdl('postgres', 'CREATE TABLE public.users (id integer);');
    const ns = toSqlNamespace(schema) as Record<string, unknown>;
    expect(ns.users).toBeDefined();
    expect((ns.public as Record<string, unknown>).users).toBeDefined();
  });

  test('a view completes with no columns', () => {
    const schema = parseDdl('postgres', 'CREATE VIEW active_users AS SELECT * FROM users;');
    const ns = toSqlNamespace(schema) as Record<string, unknown>;
    expect(ns.active_users).toEqual([]);
  });
});

// namespaceFromCached is the metadata-cache analogue of toSqlNamespace above — a
// shape transform with the same interacting rules (a view and a table must not cross-contaminate
// each other's columns; an empty relation list or an empty column list must produce a namespace
// entry sqlSchemaCompletion.ts can still consume; PK-first ordering) feeding the same completion
// engine, which is why it earns the same unit coverage toSqlNamespace already has.
describe('namespaceFromCached — P22c D4: RelationColumns[] -> SchemaNamespace', () => {
  function col(overrides: Partial<RelationColumns['columns'][number]> = {}) {
    return {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
      ...overrides,
    };
  }

  test('each relation completes under its own bare name, unqualified', () => {
    const relations: RelationColumns[] = [
      { name: 'users', kind: 'table', columns: [col({ name: 'id' }), col({ name: 'email' })] },
    ];
    const ns = namespaceFromCached(relations, 'postgres') as Record<string, unknown>;
    expect(Object.keys(ns)).toEqual(['users']);
    expect((ns.users as { label: string }[]).map((c) => c.label)).toEqual(['id', 'email']);
  });

  test('a view and a table with the same column name do not cross-contaminate', () => {
    const relations: RelationColumns[] = [
      { name: 'users', kind: 'table', columns: [col({ name: 'id' })] },
      { name: 'active_users', kind: 'view', columns: [col({ name: 'id', dataType: 'bigint' })] },
    ];
    const ns = namespaceFromCached(relations, 'postgres') as Record<
      string,
      { label: string; detail?: string }[]
    >;
    expect(ns.users?.[0]?.detail).toBe('integer');
    expect(ns.active_users?.[0]?.detail).toBe('bigint');
  });

  test('an empty relation list produces an empty namespace', () => {
    expect(namespaceFromCached([], 'postgres')).toEqual({});
  });

  test('a relation with no columns still gets a (empty) namespace entry', () => {
    const relations: RelationColumns[] = [{ name: 'empty_view', kind: 'view', columns: [] }];
    const ns = namespaceFromCached(relations, 'postgres') as Record<string, unknown[]>;
    expect(ns.empty_view).toEqual([]);
  });

  test('a primary-key column sorts first, mirroring toSqlNamespace/D10', () => {
    const relations: RelationColumns[] = [
      {
        name: 'orders',
        kind: 'table',
        columns: [col({ name: 'total' }), col({ name: 'id', isPrimaryKey: true })],
      },
    ];
    const ns = namespaceFromCached(relations, 'postgres') as Record<
      string,
      { label: string; boost?: number }[]
    >;
    const byLabel = Object.fromEntries((ns.orders ?? []).map((c) => [c.label, c.boost ?? 0]));
    expect(byLabel.id).toBe(1);
    expect(byLabel.total).toBe(0);
  });

  // A column needing quotes (a reserved word, here) gets `insert` set to its quoted form
  // (EditorCompletion's own field, the replacement for lang-sql's `Completion.apply`) — label
  // stays bare so matching/highlighting still works — the same identNeedsQuoting/quoteIdent rule
  // views/grid/filterCompletion.ts's own column completions already follow.
  test('a column needing quotes gets a quoted `insert`, mysql backtick vs. postgres double-quote', () => {
    const relations: RelationColumns[] = [
      { name: 'orders', kind: 'table', columns: [col({ name: 'order' }), col({ name: 'total' })] },
    ];
    const pg = namespaceFromCached(relations, 'postgres') as Record<
      string,
      { label: string; insert?: string }[]
    >;
    const pgByLabel = Object.fromEntries((pg.orders ?? []).map((c) => [c.label, c.insert]));
    expect(pgByLabel.order).toBe('"order"');
    expect(pgByLabel.total).toBeUndefined();

    const mysql = namespaceFromCached(relations, 'mysql') as Record<
      string,
      { label: string; insert?: string }[]
    >;
    const mysqlByLabel = Object.fromEntries((mysql.orders ?? []).map((c) => [c.label, c.insert]));
    expect(mysqlByLabel.order).toBe('`order`');
  });
});

// The old block ("dialect identifierQuotes vs. sqlIdent.ts quoteIdent") tested whether
// lang-sql's own per-dialect `identifierQuotes` spec agreed with this app's own quoteIdent
// (sqlIdent.ts) — a real risk when a third-party library's quoting and this app's own could drift.
// The library was removed: `sqlSchemaCompletion.ts`'s completion accept now calls quoteIdent
// directly (sqlIdent.ts's BACKTICK_DIALECTS), the only quoting authority left, so there is no
// second opinion left to disagree with. Deleted, not ported — the property it guarded no longer
// exists to break.

describe('defaultSchemaFor', () => {
  test("postgres: 'public' only when the DDL actually qualifies a table with it", () => {
    const withPublic = parseDdl('postgres', 'CREATE TABLE public.users (id integer);');
    expect(defaultSchemaFor(withPublic, 'postgres', undefined)).toBe('public');
    const withoutPublic = parseDdl('postgres', 'CREATE TABLE users (id integer);');
    expect(defaultSchemaFor(withoutPublic, 'postgres', undefined)).toBeUndefined();
  });

  test("mysql: the connection's own database, only when the DDL qualifies with it", () => {
    const schema = parseDdl('mysql', 'CREATE TABLE app.orders (id INT);');
    expect(defaultSchemaFor(schema, 'mysql', 'app')).toBe('app');
    expect(defaultSchemaFor(schema, 'mysql', 'other')).toBeUndefined();
  });
});
