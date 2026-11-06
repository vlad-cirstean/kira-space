// MonacoHost.vue's hover provider renders every ConsoleHoverInfo/HoverInfo `lines`
// entry as a Markdown string — `supportHtml: false` blocks raw HTML only, Markdown syntax still
// rendered. `lines` carries database content this app doesn't control (table/column names, a
// column's `COMMENT ON COLUMN` description, a resolved variable's value's own explanation line), so
// a comment reading `[docs](https://…)` became a clickable link and `_x_`/`__init__`-style names
// rendered as italic/bold. `value` was wrapped in a fixed triple-backtick fence, so a value
// containing its own triple backtick closed the fence early and spilled the rest as Markdown.
import { describe, expect, test } from 'bun:test';
import {
  escapeMarkdownSyntaxTokens,
  fenceMarkdownValue,
} from '../../../frontend/src/studio/editor/hoverInfo';

describe('escapeMarkdownSyntaxTokens', () => {
  test('a Markdown link in a column comment no longer parses as a link', () => {
    expect(escapeMarkdownSyntaxTokens('see [docs](https://example.com)')).toBe(
      'see \\[docs\\]\\(https://example\\.com\\)',
    );
  });

  test('underscore-wrapped and double-underscore names no longer render as italic/bold', () => {
    expect(escapeMarkdownSyntaxTokens('_x_')).toBe('\\_x\\_');
    expect(escapeMarkdownSyntaxTokens('__init__')).toBe('\\_\\_init\\_\\_');
  });

  test('a literal backslash is escaped too, so it cannot combine with the next escape', () => {
    expect(escapeMarkdownSyntaxTokens('a\\*b')).toBe('a\\\\\\*b');
  });

  test('every listed token is escaped, and plain text is untouched', () => {
    expect(escapeMarkdownSyntaxTokens('a`*_{}[]()#+-.!|<>~b')).toBe(
      'a\\`\\*\\_\\{\\}\\[\\]\\(\\)\\#\\+\\-\\.\\!\\|\\<\\>\\~b',
    );
    expect(escapeMarkdownSyntaxTokens('plain text 123')).toBe('plain text 123');
  });
});

describe('fenceMarkdownValue', () => {
  test('a value with no backticks gets the minimum 3-backtick fence', () => {
    expect(fenceMarkdownValue('hello')).toBe('```\nhello\n```');
  });

  test('a value containing a triple backtick gets a 4-backtick fence, not closed early', () => {
    const value = 'text with ``` inside';
    const fenced = fenceMarkdownValue(value);
    expect(fenced).toBe('````\ntext with ``` inside\n````');
    // The fence itself is the only 4-backtick run — CommonMark closes on a fence-length-or-longer
    // run, so the embedded ``` (length 3) can't terminate it early.
    expect(fenced.match(/`{4,}/g)).toHaveLength(2);
  });

  test('the fence is one longer than the longest backtick run inside, not just +1 over 3', () => {
    const value = 'a ````` run'; // 5 backticks inside
    expect(fenceMarkdownValue(value)).toBe('``````\na ````` run\n``````');
  });
});
