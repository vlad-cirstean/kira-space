import { expect, test } from 'bun:test';
import {
  beautifyJson,
  beautifyXml,
  scanJson,
  scanXml,
} from '../../../frontend/src/studio/beautify';

const DEPTH = 200_000;

test('extreme nesting is refused, not thrown', () => {
  const json = '['.repeat(DEPTH) + ']'.repeat(DEPTH);
  expect(scanJson(json).ok).toBe(false);
  expect(beautifyJson(json, 'indented').ok).toBe(false);
  const xml = '<a>'.repeat(DEPTH) + '</a>'.repeat(DEPTH);
  expect(() => {
    scanXml(xml);
    beautifyXml(xml, 'indented');
  }).not.toThrow();
});
