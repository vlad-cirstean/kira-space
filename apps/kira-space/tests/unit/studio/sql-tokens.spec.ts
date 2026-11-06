// sql-tokens.ts's own contract — the scanner and its two grouping passes. Earns a
// unit test per CLAUDE.md's bar (interacting lexical-state rules, boundary arithmetic): nested
// Parens; a `;`/quote/comment interaction; unterminated string/comment/paren never throwing;
// CompositeIdentifier runs; the MySQL-vs-Postgres double-quote split; backslashEscapes on/off.
import { describe, expect, test } from 'bun:test';
import { keywordsFor } from '@shared/domain/sqlKeywords';
import { type SqlTokenOptions, tokenizeSql } from '@shared/domain/sqlTokens';

function opts(overrides: Partial<SqlTokenOptions> = {}): SqlTokenOptions {
  return {
    backslashEscapes: false,
    dollarQuoting: true,
    keywords: keywordsFor('postgres'),
    identifierQuotes: '"',
    ...overrides,
  };
}

function names(node: ReturnType<typeof tokenizeSql>): string[] {
  const out: string[] = [];
  let c = node.firstChild;
  while (c) {
    out.push(c.name);
    c = c.nextSibling;
  }
  return out;
}

function statementsOf(root: ReturnType<typeof tokenizeSql>) {
  let c = root.firstChild;
  const out: NonNullable<typeof c>[] = [];
  while (c) {
    if (c.name === 'Statement') out.push(c);
    c = c.nextSibling;
  }
  return out;
}

describe('tokenizeSql — statement grouping', () => {
  test('two statements split on top-level ;, each keeping its own terminator', () => {
    const root = tokenizeSql('SELECT 1; SELECT 2;', opts());
    const stmts = statementsOf(root);
    expect(stmts).toHaveLength(2);
    expect(names(stmts[0] as NonNullable<(typeof stmts)[0]>)).toEqual(['Keyword', 'Number', ';']);
  });

  test('a trailing statement with no ; still gets a Statement node', () => {
    const root = tokenizeSql('SELECT 1; SELECT 2', opts());
    expect(statementsOf(root)).toHaveLength(2);
  });

  test('a `;` inside a string literal is not a statement boundary', () => {
    const root = tokenizeSql("SELECT 'a;b'; SELECT 2;", opts());
    expect(statementsOf(root)).toHaveLength(2);
  });

  test('a `;` inside a line comment is not a statement boundary', () => {
    const root = tokenizeSql('SELECT 1 -- comment; still one statement\n;', opts());
    expect(statementsOf(root)).toHaveLength(1);
  });

  test('a `;` inside a block comment is not a statement boundary', () => {
    const root = tokenizeSql('SELECT 1 /* a; b */;', opts());
    expect(statementsOf(root)).toHaveLength(1);
  });

  test('a `;` inside a dollar-quoted body is not a statement boundary', () => {
    const root = tokenizeSql('SELECT $$a;b$$;', opts());
    expect(statementsOf(root)).toHaveLength(1);
  });

  test('a `;` inside parens is not a statement boundary', () => {
    const root = tokenizeSql('SELECT f(1; 2);', opts());
    expect(statementsOf(root)).toHaveLength(1);
  });

  test('two ;; in a row emit no empty Statement between them', () => {
    const root = tokenizeSql('SELECT 1;;SELECT 2;', opts());
    expect(statementsOf(root)).toHaveLength(2);
  });
});

describe('tokenizeSql — Parens nesting', () => {
  test('a nested paren group is a Parens child of the outer Parens', () => {
    const root = tokenizeSql('SELECT f(g(1, 2));', opts());
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    const outerParens = [
      ...(function* () {
        let c = stmt.firstChild;
        while (c) {
          yield c;
          c = c.nextSibling;
        }
      })(),
    ].find((n) => n.name === 'Parens');
    if (!outerParens) throw new Error('expected an outer Parens');
    expect(names(outerParens)).toEqual(['(', 'Identifier', 'Parens', ')']);
    const inner = names(outerParens).includes('Parens');
    expect(inner).toBe(true);
  });

  test('an unterminated `(` never throws — emits a Parens ending at EOF', () => {
    expect(() => tokenizeSql('SELECT f(1, 2', opts())).not.toThrow();
    const root = tokenizeSql('SELECT f(1, 2', opts());
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    let parens: { name: string; to: number } | undefined;
    let c = stmt.firstChild;
    while (c) {
      if (c.name === 'Parens') parens = c;
      c = c.nextSibling;
    }
    if (!parens) throw new Error('expected a Parens node');
    expect(parens.to).toBe('SELECT f(1, 2'.length);
  });
});

describe('tokenizeSql — never throws on unterminated anything (R2)', () => {
  test('unterminated string', () => {
    expect(() => tokenizeSql("SELECT 'abc", opts())).not.toThrow();
  });
  test('unterminated block comment', () => {
    expect(() => tokenizeSql('SELECT 1 /* abc', opts())).not.toThrow();
  });
  test('unterminated dollar-quote', () => {
    expect(() => tokenizeSql('SELECT $$abc', opts())).not.toThrow();
  });
});

describe('tokenizeSql — CompositeIdentifier grouping', () => {
  test('a two-segment qualified name groups as one CompositeIdentifier', () => {
    const root = tokenizeSql('SELECT a.b;', opts());
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    expect(names(stmt)).toEqual(['Keyword', 'CompositeIdentifier', ';']);
    const composite = stmt.firstChild?.nextSibling;
    if (!composite) throw new Error('expected a composite identifier');
    expect(names(composite)).toEqual(['Identifier', '.', 'Identifier']);
  });

  test('a three-segment qualified name groups as one CompositeIdentifier', () => {
    const root = tokenizeSql('SELECT a.b.c;', opts());
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    const composite = stmt.firstChild?.nextSibling;
    if (!composite) throw new Error('expected a composite identifier');
    expect(names(composite)).toEqual(['Identifier', '.', 'Identifier', '.', 'Identifier']);
  });

  test('a CompositeIdentifier groups inside a Parens too', () => {
    const root = tokenizeSql('SELECT f(a.b);', opts());
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    let parens: { firstChild: ReturnType<typeof tokenizeSql>['firstChild'] } | undefined;
    let c = stmt.firstChild;
    while (c) {
      if (c.name === 'Parens') parens = c;
      c = c.nextSibling;
    }
    if (!parens) throw new Error('expected a Parens node');
    const inner = parens.firstChild?.nextSibling; // skip '('
    expect(inner?.name).toBe('CompositeIdentifier');
  });
});

describe('tokenizeSql — MySQL double-quote is a String, Postgres double-quote is a QuotedIdentifier', () => {
  test('postgres: "x" is a QuotedIdentifier', () => {
    const root = tokenizeSql('SELECT "x";', opts({ identifierQuotes: '"' }));
    const stmt = statementsOf(root)[0];
    expect(stmt?.firstChild?.nextSibling?.name).toBe('QuotedIdentifier');
  });

  test('mysql: "x" is a String, `x` is a QuotedIdentifier', () => {
    const root = tokenizeSql('SELECT "x", `y`;', opts({ identifierQuotes: '`' }));
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    expect(names(stmt)).toEqual(['Keyword', 'String', 'Punctuation', 'QuotedIdentifier', ';']);
  });
});

describe('tokenizeSql — backslashEscapes on/off', () => {
  test('on (mysql/clickhouse): a backslash escapes the next character, string runs past a lone quote', () => {
    const root = tokenizeSql("SELECT 'a\\'b';", opts({ backslashEscapes: true }));
    const stmt = statementsOf(root)[0];
    expect(stmt?.firstChild?.nextSibling?.name).toBe('String');
    expect(stmt?.firstChild?.nextSibling?.to).toBe("SELECT 'a\\'b'".length);
  });

  test('off (postgres/sqlite): a backslash is an ordinary character, the string ends at the next quote', () => {
    const root = tokenizeSql("SELECT 'a\\'||'b';", opts({ backslashEscapes: false }));
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    // "'a\'" ends at the quote right after the backslash — a String, then an Operator (||), then
    // a second String, not one long escaped run.
    expect(names(stmt)).toEqual(['Keyword', 'String', 'Operator', 'String', ';']);
  });
});

describe('tokenizeSql — keywords, numbers, null', () => {
  test('a word in the keyword set is Keyword; NULL (any case) is Null; anything else is Identifier', () => {
    const root = tokenizeSql('SELECT id, NULL, Null, foo FROM t;', opts());
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    expect(names(stmt)).toEqual([
      'Keyword', // SELECT
      'Identifier', // id (F5.1: not a keyword in this repo's curated postgres set)
      'Punctuation', // ,
      'Null',
      'Punctuation',
      'Null',
      'Punctuation',
      'Identifier', // foo
      'Keyword', // FROM
      'Identifier', // t
      ';',
    ]);
  });

  test('numbers: integer, decimal, exponent, hex', () => {
    const root = tokenizeSql('SELECT 1, 1.5, 1e6, 1E-6, 0x1F;', opts());
    const stmt = statementsOf(root)[0];
    if (!stmt) throw new Error('expected a statement');
    const numberTexts = [] as string[];
    let c = stmt.firstChild;
    const source = 'SELECT 1, 1.5, 1e6, 1E-6, 0x1F;';
    while (c) {
      if (c.name === 'Number') numberTexts.push(source.slice(c.from, c.to));
      c = c.nextSibling;
    }
    expect(numberTexts).toEqual(['1', '1.5', '1e6', '1E-6', '0x1F']);
  });
});
