// Real-interaction fix (reported bug — the {{variable}} hover tooltip "looks horrible": the value
// and its explanation ran together, all in one undifferentiated block of monospace lines, and a
// JSON-shaped value rendered as a single truncated line rather than something readable).
//
// This file used to also unit-test `editor/hover.ts`'s own `buildHoverSource` — the
// CodeMirror-specific glue that turned a `ConsoleHoverInfo` into a `Tooltip`'s DOM. That glue is
// gone (`MonacoHost.vue`'s own hover provider does the equivalent mapping straight into Monaco's
// `MarkdownString[]` contents) — a thin, mechanical transform inside a Vue component,
// which this app's own convention doesn't unit-test (no `.vue` file in this repo has a dedicated
// spec; UI specs cover that layer instead — `tests/ui/api-ui-consistency.spec.ts`'s own "shows its
// value" hover check already exercises the value/caption split at the UI level). `formatHoverValue`
// is the one piece of real logic left here, and it still earns its own test.
import { describe, expect, test } from 'bun:test';
import { formatHoverValue } from '../../../frontend/src/studio/editor/hoverInfo';

describe('formatHoverValue — pretty-prints a JSON-shaped value, leaves anything else verbatim', () => {
  test('a JSON object re-indents', () => {
    expect(formatHoverValue('{"a":1,"b":"x"}')).toBe('{\n  "a": 1,\n  "b": "x"\n}');
  });

  test('a JSON array re-indents', () => {
    expect(formatHoverValue('[1,2,3]')).toBe('[\n  1,\n  2,\n  3\n]');
  });

  test('a bare string value (the common case — most variables are not JSON) is unchanged', () => {
    expect(formatHoverValue('https://api.example.com')).toBe('https://api.example.com');
  });

  test('invalid/non-JSON text is unchanged, not a throw', () => {
    expect(() => formatHoverValue('not { json at all')).not.toThrow();
    expect(formatHoverValue('not { json at all')).toBe('not { json at all');
  });
});
