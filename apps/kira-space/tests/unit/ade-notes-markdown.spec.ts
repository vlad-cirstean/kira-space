import { describe, expect, test } from 'bun:test';
import { getSchema } from '@tiptap/core';
import { MarkdownManager } from '@tiptap/markdown';
import { parseWithLineBreaks } from '../../frontend/src/ade/v2/notes/notesDoc';
import { notesExtensions } from '../../frontend/src/ade/v2/notes/notesExtensions';

// A headless Markdown round-trip against the shared extension set
// (`notesExtensions.ts`) — no DOM (`bun test` has none), so no `@tiptap/vue-3` editor instance:
// `MarkdownManager` parses/serializes directly, and `getSchema(...).nodeFromJSON` proves the parsed
// JSON is a schema-valid document (catches an extension wired wrong before it ever reaches
// `AdeNotesEditor.vue`).
const manager = new MarkdownManager({ extensions: notesExtensions() });
const schema = getSchema(notesExtensions());

/** Parses `md`, validates the result against the schema, and returns the re-serialized Markdown. */
function roundTrip(md: string): string {
  const json = manager.parse(md);
  schema.nodeFromJSON(json); // throws on an invalid document
  return manager.serialize(json);
}

describe('ade-notes-markdown', () => {
  test('heading', () => {
    expect(roundTrip('## heading')).toBe('## heading');
  });

  test('bulleted list', () => {
    expect(roundTrip('- a')).toBe('- a');
  });

  test('numbered list', () => {
    expect(roundTrip('1. a')).toBe('1. a');
  });

  test('checklist: unchecked and checked', () => {
    expect(roundTrip('- [ ] a')).toBe('- [ ] a');
    expect(roundTrip('- [x] b')).toBe('- [x] b');
  });

  test('bold', () => {
    expect(roundTrip('**bold**')).toBe('**bold**');
  });

  test('italic', () => {
    expect(roundTrip('*italic*')).toBe('*italic*');
  });

  test('code', () => {
    expect(roundTrip('`code`')).toBe('`code`');
  });

  test('link', () => {
    expect(roundTrip('[text](https://x.test/a)')).toBe('[text](https://x.test/a)');
  });

  test('nested marks inside a task item', () => {
    const md = '- [ ] **a** *b* [l](https://x.test)';
    expect(roundTrip(md)).toBe(md);
  });

  test('mixed document (mockup billing note, line 573)', () => {
    // The mockup's own single `\n` between the heading and the list isn't a fixed point (a
    // heading needs a blank line before a following block to stay canonical) — this is that note
    // re-serialized once, the "canonical input" every other case in this file already is.
    const md =
      '## Q4 pricing launch\n\n' +
      '- [x] Usage metering\n' +
      '- [ ] Invoice lines **before** Oct 1\n' +
      '- [ ] Align with *Sara* on merge timing\n\n' +
      'Totals live in `invoice.ts` for now.';
    expect(roundTrip(md)).toBe(md);
  });

  test('a checklist item toggled via JSON serializes the new state', () => {
    const json = manager.parse('- [ ] a');
    // biome-ignore lint/suspicious/noExplicitAny: TipTap's own JSONContent leaves list content loosely typed
    (json as any).content[0].content[0].attrs.checked = true;
    expect(manager.serialize(json)).toBe('- [x] a');
  });

  test('parse is idempotent', () => {
    const md =
      '## Q4 pricing launch\n\n- [x] Usage metering\n- [ ] Invoice lines **before** Oct 1\n\n' +
      'Totals live in `invoice.ts` for now.';
    const once = roundTrip(md);
    expect(roundTrip(once)).toBe(once);
  });

  test('parseWithLineBreaks keeps single newlines as line breaks; the plain parse folds them', () => {
    const md = 'Form wired.\nValidation missing.';
    expect(JSON.stringify(parseWithLineBreaks(md))).toContain('hardBreak');
    expect(JSON.stringify(manager.parse(md))).not.toContain('hardBreak');
    const code = '```\na\nb\n```';
    expect(JSON.stringify(parseWithLineBreaks(code))).not.toContain('hardBreak');
  });
});
