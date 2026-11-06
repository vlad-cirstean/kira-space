// Cursor context of a KQL query for completion: the lexer and the clause state machine interact
// (quotes, nesting, value lists, negation), so the positions are pinned here.
import { describe, expect, test } from 'bun:test';
import { kqlCompletionContext } from '../../../frontend/src/studio/views/console/kibana/kqlContext';

function at(marked: string) {
  const offset = marked.indexOf('|');
  const doc = marked.replace('|', '');
  return kqlCompletionContext(doc, offset);
}

describe('kqlCompletionContext', () => {
  test('field positions', () => {
    expect(at('|')).toMatchObject({ kind: 'field', prefix: '', nested: null });
    expect(at('lev|')).toMatchObject({ kind: 'field', prefix: 'lev', range: { from: 0, to: 3 } });
    expect(at('a:1 and |')).toMatchObject({ kind: 'field', prefix: '' });
    expect(at('a:1 or (b|')).toMatchObject({ kind: 'field', prefix: 'b' });
    expect(at('not |')).toMatchObject({ kind: 'field' });
  });

  test('operator after a field and a space', () => {
    expect(at('status |')).toEqual({
      kind: 'operator',
      field: 'status',
      range: { from: 7, to: 7 },
    });
    expect(at('status|')).toMatchObject({ kind: 'field', prefix: 'status' });
  });

  test('value positions', () => {
    expect(at('level:|')).toMatchObject({
      kind: 'value',
      field: 'level',
      prefix: '',
      quoted: false,
    });
    expect(at('level:err|')).toMatchObject({
      kind: 'value',
      prefix: 'err',
      range: { from: 6, to: 9 },
    });
    expect(at('status >= |')).toMatchObject({ kind: 'value', field: 'status' });
    expect(at('level:"fo|')).toMatchObject({
      kind: 'value',
      prefix: 'fo',
      quoted: true,
      range: { from: 6, to: 9 },
    });
  });

  test('value list keeps the field across or/and and not', () => {
    expect(at('level:(error or |')).toMatchObject({ kind: 'value', field: 'level' });
    expect(at('level:(not |')).toMatchObject({ kind: 'value', field: 'level' });
    expect(at('level:(error |')).toMatchObject({ kind: 'conjunction' });
    expect(at('level:(error) and |')).toMatchObject({ kind: 'field' });
  });

  test('conjunction after a complete clause', () => {
    expect(at('level:error |')).toMatchObject({ kind: 'conjunction', prefix: '' });
    expect(at('level:error an|')).toMatchObject({ kind: 'conjunction', prefix: 'an' });
    expect(at('level:error|')).toMatchObject({ kind: 'value', prefix: 'error' });
  });

  test('nested field scope', () => {
    expect(at('items:{ |')).toMatchObject({ kind: 'field', nested: 'items' });
    expect(at('items:{ name:x } and |')).toMatchObject({ kind: 'field', nested: null });
  });

  test('queryWithoutValue drops the clause under the cursor and stays balanced', () => {
    expect(at('a:1 and level:er|')).toMatchObject({ queryWithoutValue: 'a:1' });
    expect(at('level:er|')).toMatchObject({ queryWithoutValue: '' });
    expect(at('(a:1 or level:er|')).toMatchObject({ queryWithoutValue: '' });
    expect(at('a:"x y" and level:(er|')).toMatchObject({ queryWithoutValue: 'a:"x y"' });
  });

  test('an escaped quote does not end a quoted value', () => {
    expect(at('msg:"say \\"hi|')).toMatchObject({ kind: 'value', prefix: 'say "hi', quoted: true });
  });
});
