import { expect, test } from 'bun:test';
import { renderMarkdown } from '../../frontend/src/hostview/markdown';

test('alert custom title is escaped, never raw HTML', () => {
  const html = renderMarkdown('> [!NOTE] <img src=x onerror=alert(1)>\n> body');
  expect(html).not.toContain('<img');
  expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;');
});
