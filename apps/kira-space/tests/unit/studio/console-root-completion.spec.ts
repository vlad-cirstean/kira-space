// A console opened at a connection's ROOT (path === '') has no database:/schema: segment for
// containerPathFor/consoleRelationNames to walk to — both returned null/[] immediately, so a root
// console (the ordinary "right-click a connection → Open query console" action) got no
// schema-aware completion at all, even when the tree already had everything it needed cached.
// Neither function had any test coverage before this file (the census found zero —
// tests/unit/autocomplete-tokenizers.spec.ts, despite its name, covers only theme/completion.ts's
// plain-field tokenizers).
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { TreeNode } from '@shared/domain/tree';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { containerPathFor } = await import('../../../frontend/src/studio/state/schemaColumns');
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTreeStore } = await import('../../../frontend/src/studio/project/state/tree');
const treeStore = useTreeStore();
const { consoleRelationNames } = await import('../../../frontend/src/studio/views/console/completion');

function addConnection(id: string, kind: string, database: string | null): void {
  connectionsStore.records.push({
    id,
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
    ...({ kind, database, name: id, color: 'blue' } as any),
  } as ConnectionSummary);
}

function dbNode(name: string, connected: boolean): TreeNode {
  return {
    kind: 'database',
    name,
    path: `database:${name}`,
    hasChildren: true,
    detail: connected ? 'connected' : undefined,
  };
}

function schemaNode(name: string): TreeNode {
  return { kind: 'schema', name, path: `schema:${name}`, hasChildren: true };
}

function tableNode(name: string): TreeNode {
  return { kind: 'table', name, path: `table:${name}`, hasChildren: false };
}

describe('containerPathFor — root branch (P4)', () => {
  test('postgres: resolves via the "connected" marker + schema:public', () => {
    const id = 'conn-pg-connected';
    addConnection(id, 'postgres', 'other_db');
    treeStore.children[`${id}|`] = [dbNode('other_db', false), dbNode('kira_test', true)];
    treeStore.children[`${id}|database:kira_test`] = [schemaNode('internal'), schemaNode('public')];

    expect(containerPathFor(id, '')).toBe('database:kira_test/schema:public');
  });

  test('postgres: falls back to the connection record\'s own database name with no "connected" marker', () => {
    const id = 'conn-pg-record-db';
    addConnection(id, 'postgres', 'kira_test');
    treeStore.children[`${id}|`] = [dbNode('other_db', false), dbNode('kira_test', false)];
    treeStore.children[`${id}|database:kira_test`] = [schemaNode('public')];

    expect(containerPathFor(id, '')).toBe('database:kira_test/schema:public');
  });

  test('postgres: no "connected" marker and multiple non-matching databases -> null', () => {
    const id = 'conn-pg-ambiguous';
    addConnection(id, 'postgres', 'does_not_exist');
    treeStore.children[`${id}|`] = [dbNode('a', false), dbNode('b', false)];

    expect(containerPathFor(id, '')).toBeNull();
  });

  test('postgres: sole non-system schema is used when public is absent', () => {
    const id = 'conn-pg-sole-schema';
    addConnection(id, 'postgres', 'kira_test');
    treeStore.children[`${id}|`] = [dbNode('kira_test', true)];
    treeStore.children[`${id}|database:kira_test`] = [schemaNode('app')];

    expect(containerPathFor(id, '')).toBe('database:kira_test/schema:app');
  });

  test('postgres: ambiguous schema (no public, more than one) resolves to null, not the database alone', () => {
    // A database-only path is rejected outright by every postgres call this feeds
    // (SchemaColumns requires database:X/schema:Y exactly) — stopping there would be a
    // functionally dead container, not a partial win.
    const id = 'conn-pg-ambiguous-schema';
    addConnection(id, 'postgres', 'kira_test');
    treeStore.children[`${id}|`] = [dbNode('kira_test', true)];
    treeStore.children[`${id}|database:kira_test`] = [schemaNode('a'), schemaNode('b')];

    expect(containerPathFor(id, '')).toBeNull();
  });

  test('mysql: resolves via the "connected" marker, one level, no schema', () => {
    const id = 'conn-mysql';
    addConnection(id, 'mysql', 'app');
    treeStore.children[`${id}|`] = [dbNode('sys', false), dbNode('app', true)];

    expect(containerPathFor(id, '')).toBe('database:app');
  });

  test('clickhouse: no "connected" marker, resolves via the record\'s own database name', () => {
    const id = 'conn-clickhouse';
    addConnection(id, 'clickhouse', 'analytics');
    treeStore.children[`${id}|`] = [dbNode('default', false), dbNode('analytics', false)];

    expect(containerPathFor(id, '')).toBe('database:analytics');
  });

  test("sqlite: the record's database is a file path, resolved via the sole database child instead", () => {
    const id = 'conn-sqlite';
    addConnection(id, 'sqlite', '/Users/me/app.db');
    treeStore.children[`${id}|`] = [dbNode('main', false)];

    expect(containerPathFor(id, '')).toBe('database:main');
  });

  test('connection never expanded (no tree children at all) -> null, same as today', () => {
    const id = 'conn-unexpanded';
    addConnection(id, 'postgres', 'kira_test');

    expect(containerPathFor(id, '')).toBeNull();
  });

  test('a non-root path is completely unaffected by the root branch', () => {
    const id = 'conn-non-root';
    addConnection(id, 'postgres', 'kira_test');
    // Deliberately no tree children at all — if the root branch fired here, it would panic or
    // return something; the ordinary walk must never even consult it.
    expect(containerPathFor(id, 'database:x/schema:y/table:z')).toBe('database:x/schema:y');
  });
});

describe('consoleRelationNames — root branch (P4)', () => {
  test('unions relation names across every already-loaded container for the connection, deduped', () => {
    const id = 'conn-union';
    treeStore.children[`${id}|`] = [dbNode('kira_test', true)];
    treeStore.children[`${id}|database:kira_test/schema:app`] = [
      tableNode('order_items'),
      tableNode('customers'),
    ];
    treeStore.children[`${id}|database:kira_test/schema:reporting`] = [
      tableNode('customers'), // same name, different container — deduped
      tableNode('daily_totals'),
    ];

    expect(new Set(consoleRelationNames(id, ''))).toEqual(
      new Set(['order_items', 'customers', 'daily_totals']),
    );
  });

  test('nothing loaded for the connection -> []', () => {
    expect(consoleRelationNames('conn-nothing-loaded', '')).toEqual([]);
  });

  test('non-relation node kinds (database/schema) are never included', () => {
    const id = 'conn-kinds';
    treeStore.children[`${id}|`] = [dbNode('kira_test', true)];
    treeStore.children[`${id}|database:kira_test`] = [schemaNode('public')];

    expect(consoleRelationNames(id, '')).toEqual([]);
  });

  test('a non-root path is unaffected by the root branch (unchanged walk)', () => {
    const id = 'conn-relnames-non-root';
    treeStore.children[`${id}|database:x/schema:y`] = [tableNode('t1')];
    // A different, unrelated root entry for the same connection — must not leak in.
    treeStore.children[`${id}|`] = [dbNode('x', true)];

    expect(consoleRelationNames(id, 'database:x/schema:y')).toEqual(['t1']);
  });
});
