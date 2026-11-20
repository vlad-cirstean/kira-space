import { describe, expect, test } from 'bun:test';
import { join } from 'node:path';
import { checkDocument } from './docsRules';

const ROOT = join(import.meta.dir, '..', '..');
const FM = '---\ntitle: T\ndescription: One sentence.\nsources:\n  - README.md\n---\n\n';

function rules(body: string, isMdx = false): string[] {
  return checkDocument(FM + body, isMdx, ROOT).map((v) => v.rule);
}

const words = (n: number) => Array.from({ length: n }, (_, i) => `w${i}`).join(' ');

describe('rules fire once on a bad line', () => {
  test.each([
    ['D1', `${words(26)}.`],
    ['D2', 'Run it, just once.'],
    ['D3', 'Edit the cfg file.'],
    ['D4', 'Go -> next.'],
    ['D5', 'Done \u{1F389}.'],
    ['D6', '| a | b |\n|---|---|\n| 1 | 2 |'],
    ['D7', 'Open the repo.'],
    ['D8', 'TODO write this.'],
  ])('%s', (rule, body) => {
    expect(rules(body)).toEqual([rule]);
  });
});

describe('masking', () => {
  test('inline code, fences, URLs and MDX ESM never fire', () => {
    const body = [
      'Run `just -> repo cfg` now.',
      '',
      '```sh',
      'just repo -> TODO',
      '```',
      '',
      '[link](https://x.test/repo/dir/->)',
      '',
      '<Comp note="just repo" />',
    ].join('\n');
    expect(rules(body, true)).toEqual([]);
    expect(rules(`import X from '../repo';\n\n${body}`, true)).toEqual([]);
  });

  test('menu path greater-than is legal', () => {
    expect(rules('Open Settings > Prompts.')).toEqual([]);
  });
});

describe('sentence length', () => {
  test('26 words fail, 25 pass', () => {
    expect(rules(`${words(25)}.`)).toEqual([]);
    expect(rules(`${words(26)}.`)).toEqual(['D1']);
  });

  test('a code span counts as one word', () => {
    expect(rules(`${words(24)} \`a b c d\`.`)).toEqual([]);
  });

  test('version numbers do not split sentences', () => {
    expect(rules(`${words(10)} 1.29.1 ${words(10)} 2.38.0 ${words(2)}.`)).toEqual([]);
  });

  test('each list item and table cell is its own unit', () => {
    const items = `- ${words(20)}.\n- ${words(20)}.`;
    expect(rules(items)).toEqual([]);
  });
});

describe('aside allowance', () => {
  test('caution allows a long sentence and filler, still flags an arrow', () => {
    const aside = (inner: string) => `:::caution\n${inner}\n:::`;
    expect(rules(aside(`${words(30)}. Please read.`))).toEqual([]);
    expect(rules(aside('Go -> next.'))).toEqual(['D4']);
  });
});

describe('frontmatter', () => {
  test('missing source path fails', () => {
    const doc = '---\ntitle: T\ndescription: One.\nsources:\n  - nope/missing.md\n---\n\nText.';
    expect(checkDocument(doc, false, ROOT).map((v) => v.rule)).toEqual(['D9']);
  });

  test('two-sentence description fails', () => {
    const doc = '---\ntitle: T\ndescription: One. Two.\nsources:\n  - README.md\n---\n\nText.';
    expect(checkDocument(doc, false, ROOT).map((v) => v.rule)).toEqual(['D9']);
  });
});
