// Decision 8: the console reads a transaction's state off the statements it ran, so what ends one
// on the server must end it here. The statement lists are the ones the flow tests ran against real
// servers (contract studio-console-script): DDL commits a MySQL transaction, COMMIT AND CHAIN keeps
// one open, a failed script leaves the transaction open. The session is the connection's, shared by
// every console tab on it.
import '@workbench/testing/unit/window';

import { afterEach, describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  forgetConnectionTransactions,
  hasOpenTransaction,
  takeOpenTransaction,
  trackTransaction,
} from '../../../frontend/src/studio/views/console/transaction';

const contracts = JSON.parse(
  readFileSync(join(import.meta.dir, '../../contract/studio-console-script.json'), 'utf8'),
) as Record<string, string[]>;

const target = (connectionId: string, path = 'database:app') => ({ connectionId, path });

afterEach(() => {
  for (const id of ['a', 'b']) {
    forgetConnectionTransactions(id);
    takeOpenTransaction(`${id}1`);
    takeOpenTransaction(`${id}2`);
  }
});

describe('what a run leaves open', () => {
  test('MySQL DDL commits implicitly, so ROLLBACK after it ends nothing', () => {
    trackTransaction('a1', 'mysql', target('a'), contracts['mysql#implicitCommit'].slice(0, 3));
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('the same script on Postgres leaves it open, DDL being transactional there', () => {
    trackTransaction('a1', 'postgres', target('a'), contracts['mysql#implicitCommit'].slice(0, 3));
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('COMMIT AND CHAIN keeps the next transaction open', () => {
    trackTransaction('a1', 'mysql', target('a'), contracts['mysql#chain']);
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('a failed script applied the statements before the failure and no later one', () => {
    const failing = contracts['mysql#failing'];
    trackTransaction('a1', 'mysql', target('a'), failing, 2);
    expect(hasOpenTransaction('a1')).toBe(true);
    // Without the failure index the trailing COMMIT would wrongly count as run.
    takeOpenTransaction('a1');
    trackTransaction('a1', 'mysql', target('a'), failing);
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('a failing DDL statement still commits before it runs', () => {
    trackTransaction('a1', 'mysql', target('a'), ['BEGIN']);
    trackTransaction('a1', 'mysql', target('a'), ['CREATE TABLE broken ('], 0);
    expect(hasOpenTransaction('a1')).toBe(false);
  });
});

describe('the transaction belongs to the connection', () => {
  test('closing one console tab leaves it to the other; the last tab on the session ends it', () => {
    trackTransaction('a1', 'postgres', target('a'), ['BEGIN']);
    trackTransaction('a2', 'postgres', target('a'), ['SELECT 1']);
    expect(hasOpenTransaction('a1')).toBe(false);
    expect(takeOpenTransaction('a1')).toBeNull();
    expect(hasOpenTransaction('a2')).toBe(true);
    expect(takeOpenTransaction('a2')).toEqual(target('a'));
  });

  test('tabs on different connections do not share one', () => {
    trackTransaction('a1', 'postgres', target('a'), ['BEGIN']);
    trackTransaction('b1', 'postgres', target('b'), ['SELECT 1']);
    expect(hasOpenTransaction('a1')).toBe(true);
    expect(hasOpenTransaction('b1')).toBe(false);
  });

  test('a dropped connection ends its transaction with the server session', () => {
    trackTransaction('a1', 'postgres', target('a'), ['BEGIN']);
    forgetConnectionTransactions('a');
    expect(hasOpenTransaction('a1')).toBe(false);
  });
});
