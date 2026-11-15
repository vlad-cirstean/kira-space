// P636: MySQL/MariaDB client `DELIMITER` directive in the console splitter. Interacting rules:
// custom terminators, quote/comment hiding, directive recognition, comment-only fragments.
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { splitSqlScript, splitSqlStatements, statementAtCursor } from '@shared/domain/sqlSplit';
import { lexOptionsFor } from '../../../frontend/src/studio/views/shared/sqlIdent';

const MYSQL = lexOptionsFor('mysql');
const texts = (source: string) => splitSqlStatements(source, MYSQL).map((s) => s.text);

const contracts = JSON.parse(
  readFileSync(join(import.meta.dir, '../../contract/studio-console-script.json'), 'utf8'),
) as Record<string, string | string[]>;

describe('DELIMITER directive', () => {
  test('a trigger body under $$ is one statement without its terminator', () => {
    const src =
      'DELIMITER $$\nCREATE TRIGGER t BEFORE INSERT ON a FOR EACH ROW\nBEGIN\n  SET @a = 1;\n  SET @b = 2;\nEND$$\n';
    const out = splitSqlStatements(src, MYSQL);
    expect(out).toHaveLength(1);
    expect(out[0]?.text.endsWith('END')).toBe(true);
    expect(out[0]?.text).toContain('SET @b = 2;');
    expect(out[0]?.delimiter).toBe('$$');
  });

  test('delimiter changes mid-script and DELIMITER ; resumes ; splitting with compound tracking', () => {
    const src = [
      'SELECT 1;',
      'DELIMITER $$',
      'SELECT 2; SELECT 3$$',
      'DELIMITER //',
      'SELECT 4$$ SELECT 5//',
      'DELIMITER ;',
      'CREATE PROCEDURE p() BEGIN SELECT 6; END;',
      'SELECT 7',
    ].join('\n');
    expect(texts(src)).toEqual([
      'SELECT 1',
      'SELECT 2; SELECT 3',
      'SELECT 4$$ SELECT 5',
      'CREATE PROCEDURE p() BEGIN SELECT 6; END',
      'SELECT 7',
    ]);
    expect(splitSqlStatements(src, MYSQL).map((s) => s.delimiter)).toEqual([
      undefined,
      '$$',
      '//',
      undefined,
      undefined,
    ]);
  });

  test('the delimiter inside quotes and comments is no boundary', () => {
    const body = [
      'SELECT \'$$\', "$$", `a$$b`',
      '-- $$ line',
      '# $$ hash',
      '/* $$ block */ 1',
    ].join('\n');
    expect(texts(`DELIMITER $$\n${body}$$\nSELECT 2$$`)).toEqual([body, 'SELECT 2']);
  });

  test('DELIMITER in a comment, string, block comment or after code is no directive', () => {
    for (const src of [
      '-- DELIMITER $$\nSELECT 1; SELECT 2',
      "SELECT 'x\nDELIMITER $$\ny'; SELECT 2",
      '/* c\nDELIMITER $$\n*/ SELECT 1; SELECT 2',
      'SELECT 1; DELIMITER $$\nSELECT 2$$',
    ]) {
      expect(splitSqlScript(src, MYSQL).directives).toEqual([]);
    }
    expect(texts('SELECT 1; DELIMITER $$\nSELECT 2$$')).toEqual([
      'SELECT 1',
      'DELIMITER $$\nSELECT 2$$',
    ]);
  });

  test('keyword case, leading blanks, tab separator and CRLF', () => {
    const out = splitSqlScript(
      'select 1;\r\n  dElImItEr\t$$\r\nSELECT 2$$\r\nDELIMITER ;\r\nSELECT 3',
      MYSQL,
    );
    expect(out.directives.map((d) => d.delimiter)).toEqual(['$$', ';']);
    expect(out.statements.map((s) => s.text)).toEqual(['select 1', 'SELECT 2', 'SELECT 3']);
  });

  test('missing argument, backslash token and unterminated quote stay ordinary text', () => {
    for (const src of [
      'DELIMITER',
      'DELIMITER \nSELECT 1',
      'DELIMITER \\\\\nSELECT 1',
      "DELIMITER 'ab\nSELECT 1",
    ]) {
      const out = splitSqlScript(src, MYSQL);
      expect(out.directives).toEqual([]);
      expect(out.statements).toHaveLength(1);
    }
  });

  test('quoted token, rest of line ignored, $$; is one token', () => {
    expect(splitSqlScript("DELIMITER '$$'\nSELECT 1$$", MYSQL).directives[0]?.delimiter).toBe('$$');
    const noted = splitSqlScript('DELIMITER $$ -- note\nSELECT 1$$', MYSQL);
    expect(noted.directives[0]?.delimiter).toBe('$$');
    expect(noted.statements.map((s) => s.text)).toEqual(['SELECT 1']);
    expect(texts('DELIMITER $$;\nSELECT 1; SELECT 2$$;\nSELECT 3')).toEqual([
      'SELECT 1; SELECT 2',
      'SELECT 3',
    ]);
  });

  test('pending SQL before a directive is its own statement; comment-only text is dropped', () => {
    expect(texts('SELECT 1\nDELIMITER $$\nSELECT 2$$')).toEqual(['SELECT 1', 'SELECT 2']);
    expect(texts('-- banner\nDELIMITER $$\nSELECT 2$$')).toEqual(['SELECT 2']);
    expect(texts('SELECT 1;\n-- Dump completed')).toEqual(['SELECT 1']);
    expect(texts('SELECT 1; /* c */ # h')).toEqual(['SELECT 1']);
    expect(texts('/*!50003 SET x = 1 */;')).toEqual(['/*!50003 SET x = 1 */']);
    expect(texts('/*M!100100 SET x = 1 */;')).toEqual(['/*M!100100 SET x = 1 */']);
  });

  test('offsets index the source and never overlap a directive', () => {
    const src =
      'SELECT 1;\nDELIMITER $$\n  CREATE TRIGGER t BEGIN SET @a = 1; END  $$\nDELIMITER ;\nSELECT 2;';
    const { statements, directives } = splitSqlScript(src, MYSQL);
    for (const s of statements) {
      expect(src.slice(s.start, s.end).trim()).toBe(s.text);
      for (const d of directives) expect(s.end <= d.start || s.start >= d.end).toBe(true);
    }
    for (const d of directives) expect(src.slice(d.start, d.end)).toMatch(/^DELIMITER .*\n$/);
    const inBody = statementAtCursor(src, src.indexOf('SET @a'), MYSQL);
    expect(inBody?.text).toBe('CREATE TRIGGER t BEGIN SET @a = 1; END');
  });

  test('a multi-character delimiter inside a bare identifier splits there (client parity)', () => {
    expect(texts('DELIMITER $$\nSELECT a$$b$$')).toEqual(['SELECT a', 'b']);
  });

  test('option off: DELIMITER is ordinary text on other dialects', () => {
    const src = 'DELIMITER $$\nSELECT 1; SELECT 2$$';
    const sqlite = splitSqlScript(src, lexOptionsFor('sqlite'));
    expect(sqlite.directives).toEqual([]);
    expect(sqlite.statements.map((s) => s.text)).toEqual(['DELIMITER $$\nSELECT 1', 'SELECT 2$$']);
    // Postgres reads `$$` as a dollar quote, so nothing splits.
    const postgres = splitSqlScript(src, lexOptionsFor('postgres'));
    expect(postgres.directives).toEqual([]);
    expect(postgres.statements).toHaveLength(1);
  });

  test('contract scripts split to the statement lists the flow tests ran', () => {
    for (const key of ['delimiter', 'dump']) {
      const script = contracts[`mysql#${key}Script`] as string;
      expect(texts(script)).toEqual(contracts[`mysql#${key}Statements`] as string[]);
    }
  });

  test('op-log Re-run: a BEGIN body re-splits whole, a no-BEGIN body with inner ; does not', () => {
    // Go records the batch joined with `;\n`, no DELIMITER (architecture: Known open items).
    const begin = 'CREATE TRIGGER t BEFORE INSERT ON a FOR EACH ROW BEGIN SET @a = 1; END';
    const noBegin =
      'CREATE TRIGGER u BEFORE INSERT ON a FOR EACH ROW IF NEW.x THEN SET NEW.y = 1; END IF';
    expect(texts([begin, 'SELECT 1'].join(';\n'))).toEqual([begin, 'SELECT 1']);
    expect(texts([noBegin, 'SELECT 1'].join(';\n'))).toHaveLength(3);
  });
});
