// Decision 8: the console tracks the session's transaction so a tab close prompts and rolls back.
// Where the engine reports its state (Postgres, MariaDB, SQLite: execute response or error details)
// that state wins. MySQL 8.4 reports none, so the statement lists the flow tests ran against real
// servers (contract studio-console-script) are replayed: DDL commits unless the parser rejected it
// (1064), COMMIT AND CHAIN keeps one open, autocommit=0 reopens one after each COMMIT, XA ends with
// XA END plus XA ROLLBACK, a failed script leaves the transaction open. The session is the
// connection's, shared by every console tab on it.
import '@workbench/testing/unit/window';

import { afterEach, describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  failureOutcome,
  forgetConnectionTransactions,
  hasOpenTransaction,
  stripLeadingComments,
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
    trackTransaction('a1', 'mysql', target('a'), failing, { failed: { index: 2, errno: 1366 } });
    expect(hasOpenTransaction('a1')).toBe(true);
    // Without the failure index the trailing COMMIT would wrongly count as run.
    takeOpenTransaction('a1');
    trackTransaction('a1', 'mysql', target('a'), failing);
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('a DDL statement the parser rejects (1064) commits nothing', () => {
    const script = contracts['mysql#parseErrorDDL'];
    trackTransaction('a1', 'mysql', target('a'), script, { failed: { index: 2, errno: 1064 } });
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('a DDL statement that parses and fails later has committed', () => {
    const script = contracts['mysql#runtimeErrorDDL'];
    trackTransaction('a1', 'mysql', target('a'), script, { failed: { index: 2, errno: 1050 } });
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('a failed COMMIT leaves the transaction as it was', () => {
    trackTransaction('a1', 'mysql', target('a'), ['BEGIN', 'COMMIT'], {
      failed: { index: 1, errno: 1213 },
    });
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('a failed run that reports no index applied nothing', () => {
    trackTransaction('a1', 'mysql', target('a'), ['BEGIN'], { failed: {} });
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('CHECK TABLE commits implicitly', () => {
    trackTransaction('a1', 'mysql', target('a'), contracts['mysql#checkTable'].slice(0, 3));
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('RESET commits implicitly even when it fails on privileges (1227)', () => {
    trackTransaction('a1', 'mysql', target('a'), contracts['mysql#reset'], {
      failed: { index: 2, errno: 1227 },
    });
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('CALL may leave a transaction open, so it counts as open', () => {
    trackTransaction('a1', 'mysql', target('a'), contracts['mysql#call']);
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('USE keeps the session: the transaction stays on it', () => {
    trackTransaction('a1', 'mysql', target('a'), ['BEGIN', 'USE other_db']);
    expect(hasOpenTransaction('a1')).toBe(true);
    expect(takeOpenTransaction('a1')?.statements).toEqual(['ROLLBACK']);
  });
});

describe('autocommit=0 (MySQL)', () => {
  test('a write after COMMIT opens a transaction, and the close restores autocommit', () => {
    trackTransaction('a1', 'mysql', target('a'), contracts['mysql#autocommitOff']);
    expect(hasOpenTransaction('a1')).toBe(true);
    expect(takeOpenTransaction('a1')?.statements).toEqual(contracts['mysql#autocommitClose']);
  });

  test('autocommit off with nothing open asks nothing yet still restores autocommit', () => {
    trackTransaction('a1', 'mysql', target('a'), ['SET autocommit=0', 'COMMIT']);
    expect(hasOpenTransaction('a1')).toBe(false);
    expect(takeOpenTransaction('a1')?.statements).toEqual(['SET autocommit=1']);
  });

  test('switching autocommit on commits', () => {
    trackTransaction('a1', 'mysql', target('a'), ['SET autocommit=0', 'BEGIN', 'SET autocommit=1']);
    expect(hasOpenTransaction('a1')).toBe(false);
    expect(takeOpenTransaction('a1')).toBeNull();
  });
});

describe('XA (MySQL)', () => {
  test('an active XA transaction ends with XA END and XA ROLLBACK', () => {
    trackTransaction('a1', 'mysql', target('a'), contracts['mysql#xa']);
    expect(hasOpenTransaction('a1')).toBe(true);
    expect(takeOpenTransaction('a1')?.statements).toEqual([
      "XA END 'kira_xa'",
      "XA ROLLBACK 'kira_xa'",
    ]);
  });

  test('after XA END only the rollback is left to send', () => {
    trackTransaction('a1', 'mysql', target('a'), [...contracts['mysql#xa'], "XA END 'kira_xa'"]);
    expect(takeOpenTransaction('a1')?.statements).toEqual(["XA ROLLBACK 'kira_xa'"]);
  });

  test('XA COMMIT ends it', () => {
    trackTransaction('a1', 'mysql', target('a'), [
      "XA START 'x'",
      "XA END 'x'",
      "XA PREPARE 'x'",
      "XA COMMIT 'x'",
    ]);
    expect(hasOpenTransaction('a1')).toBe(false);
  });
});

describe('the engine reports the state', () => {
  test('Postgres: an open state wins over a statement list that shows nothing', () => {
    trackTransaction('a1', 'postgres', target('a'), ['SELECT 1'], { transaction: 'open' });
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('MariaDB: DDL, CHECK TABLE and a failed run are read from the server, not the text', () => {
    trackTransaction('a1', 'mariadb', target('a'), ['BEGIN', 'CHECK TABLE t'], {
      transaction: 'idle',
    });
    expect(hasOpenTransaction('a1')).toBe(false);
    trackTransaction('a1', 'mariadb', target('a'), ['CREATE TABL x'], {
      failed: { index: 0, errno: 1064 },
      transaction: 'open',
    });
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('a failed COMMIT that ended the transaction (Postgres deferred constraint) is idle', () => {
    trackTransaction('a1', 'postgres', target('a'), ['BEGIN']);
    trackTransaction('a1', 'postgres', target('a'), ['BEGIN', 'INSERT', 'COMMIT'], {
      failed: { index: 2 },
      transaction: 'idle',
    });
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('SQLite SAVEPOINT outside a transaction opens one, RELEASE can close it', () => {
    trackTransaction('a1', 'sqlite', target('a'), ['SAVEPOINT s1']);
    expect(hasOpenTransaction('a1')).toBe(true);
    trackTransaction('a1', 'sqlite', target('a'), ['RELEASE s1'], { transaction: 'idle' });
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('an error carries the engine state, the statement index and the server error number', () => {
    const err = Object.assign(new Error('boom'), {
      code: 'E_QUERY',
      details: { statementIndex: 2, sqlErrno: 1064, inTransaction: true },
    });
    expect(failureOutcome(err)).toEqual({
      failed: { index: 2, errno: 1064 },
      transaction: 'open',
    });
    expect(failureOutcome(new Error('x')).transaction).toBeUndefined();
  });
});

describe('comments in front of a statement', () => {
  test('a MySQL # line comment is skipped', () => {
    trackTransaction('a1', 'mysql', target('a'), ['# note\nBEGIN']);
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('# is not a comment on Postgres', () => {
    expect(stripLeadingComments('# note\nBEGIN', 'postgres')).toBe('# note\nBEGIN');
  });

  test('Postgres block comments nest', () => {
    trackTransaction('a1', 'postgres', target('a'), ['/* a /* b */ c */ BEGIN']);
    expect(hasOpenTransaction('a1')).toBe(true);
  });

  test('MySQL block comments end at the first close', () => {
    expect(stripLeadingComments('/* a /* b */ c */ BEGIN', 'mysql')).toBe('c */ BEGIN');
  });

  test('a comment hides implicit-commit DDL no longer', () => {
    trackTransaction('a1', 'mysql', target('a'), ['BEGIN', '# drop it\nDROP TABLE t']);
    expect(hasOpenTransaction('a1')).toBe(false);
  });

  test('an unterminated block comment leaves nothing', () => {
    expect(stripLeadingComments('/* never closed BEGIN', 'postgres')).toBe('');
  });
});

describe('the transaction belongs to the connection', () => {
  test('closing one console tab leaves it to the other; the last tab on the session ends it', () => {
    trackTransaction('a1', 'postgres', target('a'), ['BEGIN']);
    trackTransaction('a2', 'postgres', target('a'), ['SELECT 1']);
    expect(hasOpenTransaction('a1')).toBe(false);
    expect(takeOpenTransaction('a1')).toBeNull();
    expect(hasOpenTransaction('a2')).toBe(true);
    expect(takeOpenTransaction('a2')).toEqual({ ...target('a'), statements: ['ROLLBACK'] });
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
