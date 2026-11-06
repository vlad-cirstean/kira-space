// Where the cursor sits in a KQL query, for completion. Tolerant on purpose: it lexes only the text
// before the cursor and never throws on an unfinished query. Token rules mirror the Go lexer
// (internal/adapters/kibana/kql/lexer.go).

interface Range {
  from: number;
  to: number;
}

export type KqlContext =
  | { kind: 'field'; prefix: string; range: Range; nested: string | null }
  | { kind: 'operator'; field: string; range: Range }
  | {
      kind: 'value';
      field: string;
      prefix: string;
      quoted: boolean;
      range: Range;
      queryWithoutValue: string;
    }
  | { kind: 'conjunction'; prefix: string; range: Range }
  | { kind: 'none' };

type Tok =
  | { t: 'lparen' | 'rparen' | 'lbrace' | 'rbrace' | 'colon' | 'cmp'; from: number; to: number }
  | { t: 'and' | 'or' | 'not'; from: number; to: number }
  | { t: 'word' | 'quoted'; from: number; to: number; text: string; open: boolean };

const SPECIAL = new Set(['(', ')', ':', '<', '>', '"', '{', '}']);
const WORD_KEYWORDS = new Set(['and', 'or', 'not']);

const SIMPLE: Record<string, 'lparen' | 'rparen' | 'lbrace' | 'rbrace' | 'colon'> = {
  '(': 'lparen',
  ')': 'rparen',
  '{': 'lbrace',
  '}': 'rbrace',
  ':': 'colon',
};

function lexQuoted(src: string, i: number): Tok {
  let j = i + 1;
  let text = '';
  let closed = false;
  while (j < src.length) {
    const d = src[j] as string;
    if (d === '\\' && (src[j + 1] === '"' || src[j + 1] === '\\')) {
      text += src[j + 1];
      j += 2;
    } else if (d === '"') {
      closed = true;
      break;
    } else {
      text += d;
      j++;
    }
  }
  return { t: 'quoted', from: i, to: closed ? j + 1 : src.length, text, open: !closed };
}

function lexWord(src: string, i: number): Tok {
  let j = i;
  let text = '';
  while (j < src.length && !/\s/.test(src[j] as string) && !SPECIAL.has(src[j] as string)) {
    if (src[j] === '\\' && j + 1 < src.length) {
      text += src[j + 1];
      j += 2;
    } else {
      text += src[j];
      j++;
    }
  }
  const lower = text.toLowerCase();
  if (WORD_KEYWORDS.has(lower) && j - i === text.length) {
    return { t: lower as 'and' | 'or' | 'not', from: i, to: j };
  }
  return { t: 'word', from: i, to: j, text, open: false };
}

function lexOne(src: string, i: number): Tok {
  const c = src[i] as string;
  const simple = SIMPLE[c];
  if (simple) return { t: simple, from: i, to: i + 1 };
  if (c === '<' || c === '>') return { t: 'cmp', from: i, to: i + (src[i + 1] === '=' ? 2 : 1) };
  return c === '"' ? lexQuoted(src, i) : lexWord(src, i);
}

function lex(src: string): Tok[] {
  const toks: Tok[] = [];
  let i = 0;
  while (i < src.length) {
    if (/\s/.test(src[i] as string)) {
      i++;
      continue;
    }
    const tok = lexOne(src, i);
    toks.push(tok);
    i = tok.to;
  }
  return toks;
}

type State =
  | 'start' // expecting a clause or `not`
  | 'term' // a field or a free-text word was read
  | 'op' // `:` or a comparison was read, expecting a value
  | 'after'; // a clause is complete

interface Frame {
  kind: 'paren' | 'brace' | 'list';
  field: string;
  clauseStart: number;
  // Frame state to return to when it closes.
  state: State;
}

/** Balanced and not ending on a connective, else an empty query. */
function balanced(q: string): string {
  let depth = 0;
  let inQuote = false;
  for (let i = 0; i < q.length; i++) {
    const c = q[i];
    if (c === '\\') i++;
    else if (c === '"') inQuote = !inQuote;
    else if (!inQuote && (c === '(' || c === '{')) depth++;
    else if (!inQuote && (c === ')' || c === '}')) depth--;
  }
  return depth === 0 && !inQuote ? q : '';
}

function withoutValue(doc: string, clauseStart: number): string {
  const head = doc
    .slice(0, clauseStart)
    .replace(/\s+(and|or|not)\s*$/i, '')
    .replace(/^\s*(and|or|not)\s*$/i, '')
    .trim();
  return balanced(head);
}

interface Machine {
  state: State;
  stack: Frame[];
  field: string;
  clauseStart: number;
}

function openParen(m: Machine): void {
  const kind = m.state === 'op' ? 'list' : 'paren';
  m.stack.push({
    kind,
    field: kind === 'list' ? m.field : '',
    clauseStart: m.clauseStart,
    state: 'after',
  });
  m.state = 'start';
}

function readWord(m: Machine, tok: Extract<Tok, { t: 'word' | 'quoted' }>): void {
  if (m.state === 'start') {
    if (m.stack[m.stack.length - 1]?.kind === 'list') {
      m.state = 'after';
      return;
    }
    m.field = tok.text;
    m.clauseStart = tok.from;
    m.state = 'term';
    return;
  }
  // A value, or a second bare word (free text): either way a complete clause.
  if (m.state === 'op' || m.state === 'term') m.state = 'after';
}

function step(m: Machine, tok: Tok): void {
  switch (tok.t) {
    case 'lparen':
      openParen(m);
      break;
    case 'lbrace':
      m.stack.push({ kind: 'brace', field: m.field, clauseStart: m.clauseStart, state: 'after' });
      m.state = 'start';
      break;
    case 'rparen':
    case 'rbrace':
      m.state = m.stack.pop()?.state ?? 'after';
      break;
    case 'colon':
    case 'cmp':
      if (m.state === 'term') m.state = 'op';
      break;
    case 'and':
    case 'or':
      m.state = 'start';
      break;
    case 'not':
      // `not` negates a clause or a list value; the state stays where it was expecting one.
      if (m.state === 'after') m.state = 'start';
      break;
    case 'word':
    case 'quoted':
      readWord(m, tok);
      break;
  }
}

const TYPING = new Set(['word', 'and', 'or', 'not']);

/** The token the cursor touches is the partial word/value being typed. */
function partialToken(all: Tok[], offset: number): Tok | undefined {
  const last = all[all.length - 1];
  if (last === undefined || last.to !== offset) return undefined;
  if (last.t === 'quoted') return last.open ? last : undefined;
  return TYPING.has(last.t) ? last : undefined;
}

export function kqlCompletionContext(doc: string, offset: number): KqlContext {
  const text = doc.slice(0, offset);
  const all = lex(text);
  const partial = partialToken(all, offset);
  const m: Machine = { state: 'start', stack: [], field: '', clauseStart: 0 };
  for (const tok of partial ? all.slice(0, -1) : all) step(m, tok);

  const top = m.stack[m.stack.length - 1];
  const range: Range = { from: partial ? partial.from : offset, to: offset };
  const trailingSpace = partial === undefined && offset > 0 && /\s$/.test(text);
  let prefix = '';
  if (partial) prefix = 'text' in partial ? partial.text : text.slice(partial.from, offset);
  const quoted = partial?.t === 'quoted';
  const value = (field: string, clauseStart: number): KqlContext => ({
    kind: 'value',
    field,
    prefix,
    quoted,
    range,
    queryWithoutValue: withoutValue(doc, clauseStart),
  });

  switch (m.state) {
    case 'start':
      if (top?.kind === 'list') return value(top.field, top.clauseStart);
      return {
        kind: 'field',
        prefix,
        range,
        nested: [...m.stack].reverse().find((f) => f.kind === 'brace')?.field ?? null,
      };
    case 'term':
      // `field ` then space: an operator is next. Without the space the word is still being typed.
      return !partial && trailingSpace
        ? { kind: 'operator', field: m.field, range }
        : { kind: 'none' };
    case 'op':
      return value(m.field, m.clauseStart);
    case 'after':
      if (quoted) return { kind: 'none' };
      return partial || trailingSpace ? { kind: 'conjunction', prefix, range } : { kind: 'none' };
  }
}
