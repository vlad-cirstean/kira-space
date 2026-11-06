// P15b D3(b): wholeFieldToken and templateToken are pure, DOM-free tokenizers behind
// AutocompleteField.vue — fieldCompletion.ts is exercised from this app's tests, so this lives here rather
// than in packages/api-core/test.
import { describe, expect, test } from 'bun:test';
import {
  braceToken,
  envToken,
  templateToken,
  wholeFieldToken,
} from '@workbench/editor/fieldCompletion';

describe('wholeFieldToken (item 7 — F1: Content-T must not become Content-Content-Type)', () => {
  test('a hyphenated header name is one token, trimmed', () => {
    expect(wholeFieldToken('Content-T', 9)).toEqual({ from: 0, to: 9, word: 'Content-T' });
  });

  test('leading/trailing whitespace is trimmed for the match, but the replace range is the whole field', () => {
    expect(wholeFieldToken('  Content-T  ', 5)).toEqual({
      from: 0,
      to: 13,
      word: 'Content-T',
    });
  });

  test('the caret position does not affect the token — any caret means "the whole field"', () => {
    expect(wholeFieldToken('Content-Type', 0)).toEqual({
      from: 0,
      to: 12,
      word: 'Content-Type',
    });
  });

  test('an empty field is an empty token', () => {
    expect(wholeFieldToken('', 0)).toEqual({ from: 0, to: 0, word: '' });
  });
});

describe('templateToken (item 10 — the {{variable}} completion popup only opens inside a reference)', () => {
  test('the caret just after an unclosed {{ is inside a reference, with an empty word', () => {
    expect(templateToken('{{', 2)).toEqual({ from: 2, to: 2, word: '' });
  });

  test('the caret mid-name inside an unclosed reference', () => {
    expect(templateToken('{{ba', 4)).toEqual({ from: 2, to: 4, word: 'ba' });
  });

  test('the caret before any {{ is not inside a reference', () => {
    expect(templateToken('plain text', 5)).toBeNull();
  });

  test('the caret after a closed reference is not inside it any more', () => {
    expect(templateToken('{{name}}', 8)).toBeNull();
  });

  test('the caret inside a closed reference IS still inside it — only text after the caret matters', () => {
    // "Unclosed" is decided from the text *before* the caret only (substitute.ts's own grammar
    // reads forward from `{{`, but this tokenizer's job is "what should the popup match right
    // now", which only cares what has been typed so far).
    expect(templateToken('{{na', 4)).toEqual({ from: 2, to: 4, word: 'na' });
  });

  test('an earlier closed reference does not leak into a later, still-open one', () => {
    expect(templateToken('{{a}}xxx{{b', 11)).toEqual({ from: 10, to: 11, word: 'b' });
  });

  test('the caret right after a {{ that immediately follows a closed reference is empty, not null', () => {
    expect(templateToken('{{a}}{{', 7)).toEqual({ from: 7, to: 7, word: '' });
  });
});

// P17 D13(a): once a `|` has been typed inside the reference, the token is the run *after the
// last `|`*, not the whole `name | ...` text — this is what lets AutocompleteField offer a
// transform name without splicing over the variable name that precedes it.
describe('templateToken with a pipe (P17 D13(a))', () => {
  test('{{token | b — the word is "b", starting right after the pipe and its space', () => {
    expect(templateToken('{{token | b', 11)).toEqual({ from: 10, to: 11, word: 'b' });
  });

  test('{{token |  — no space skipped beyond the one already typed', () => {
    expect(templateToken('{{token | ', 10)).toEqual({ from: 10, to: 10, word: '' });
  });

  test('{{token |b — no space between the pipe and the letter, still tokenizes from right after the pipe', () => {
    expect(templateToken('{{token |b', 10)).toEqual({ from: 9, to: 10, word: 'b' });
  });

  test('{{token | base64 | u — the token is after the LAST pipe, for a chained pipeline', () => {
    expect(templateToken('{{token | base64 | u', 20)).toEqual({ from: 19, to: 20, word: 'u' });
  });

  test('before any pipe, the token is still the name run from {{ (unaffected by this change)', () => {
    expect(templateToken('{{tok', 5)).toEqual({ from: 2, to: 5, word: 'tok' });
  });
});

describe('braceToken (script prompt {name})', () => {
  test('identifier run after a brace is the word', () => {
    expect(braceToken('Look at {to', 11)).toEqual({ from: 9, to: 11, word: 'to' });
  });

  test('a bare brace is a non-null empty token', () => {
    expect(braceToken('a {', 3)).toEqual({ from: 3, to: 3, word: '' });
  });

  test('a closed brace before the caret is null', () => {
    expect(braceToken('{a} b', 5)).toBeNull();
  });

  test('a newline or non-identifier char between brace and caret is null', () => {
    expect(braceToken('{a\nb', 4)).toBeNull();
    expect(braceToken('{a-b', 4)).toBeNull();
    expect(braceToken('{a b', 4)).toBeNull();
  });
});

describe(`envToken (script command $NAME / \${NAME})`, () => {
  test('$ followed by an identifier run', () => {
    expect(envToken('echo $KIRA_P', 12)).toEqual({ from: 6, to: 12, word: 'KIRA_P' });
  });

  test('${ starts the token after the brace', () => {
    expect(envToken('echo ${KIRA_P', 13)).toEqual({ from: 7, to: 13, word: 'KIRA_P' });
  });

  test('bare $ and ${ are non-null empty tokens', () => {
    expect(envToken('$', 1)).toEqual({ from: 1, to: 1, word: '' });
    expect(envToken('${', 2)).toEqual({ from: 2, to: 2, word: '' });
  });

  test('plain words and a lone brace are null', () => {
    expect(envToken('echo KIRA', 9)).toBeNull();
    expect(envToken('{KIRA', 5)).toBeNull();
  });

  test('a non-identifier char after $ is null', () => {
    expect(envToken('$a-b', 4)).toBeNull();
  });
});
