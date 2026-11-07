/**
 * P284: docs style rules D1-D9 for site/src/content/docs. Mirrors docs/WORKING_AGREEMENT.md "Communication style".
 * Each rule is a pure function over prose text; `checkDocument` parses Markdown and applies them.
 */
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { Glob } from 'bun';
import remarkDirective from 'remark-directive';
import remarkFrontmatter from 'remark-frontmatter';
import remarkGfm from 'remark-gfm';
import remarkMdx from 'remark-mdx';
import remarkParse from 'remark-parse';
import { unified } from 'unified';
import { parse as parseYaml } from 'yaml';

export interface Violation {
  readonly line: number;
  readonly rule: string;
  readonly message: string;
}

/** Stands in for an inline code span: one word, never inspected. */
const CODE = String.fromCharCode(0xe000);
const MAX_WORDS = 25;

function splitSentences(text: string): string[] {
  return text
    .split(/(?<=[.?!][)"'’”]?)\s+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function wordCount(sentence: string): number {
  return sentence.split(/\s+/).filter(Boolean).length;
}

/** D1 */
function longSentences(text: string): string[] {
  return splitSentences(text)
    .filter((s) => wordCount(s) > MAX_WORDS)
    .map((s) => `D1 sentence has ${wordCount(s)} words, max ${MAX_WORDS}: "${s.slice(0, 40)}..."`);
}

const FILLER = [
  'just',
  'really',
  'basically',
  'actually',
  'simply',
  'very',
  'quite',
  'rather',
  'somewhat',
  'fairly',
  'pretty\\s+much',
  'easily',
  'obviously',
  'clearly',
  'of\\s+course',
  'certainly',
  '(?<!make\\s)sure',
  'happy\\s+to',
  'feel\\s+free',
  'please',
  'note\\s+that',
  'in\\s+order\\s+to',
  'it\\s+is\\s+worth',
  'keep\\s+in\\s+mind',
  'as\\s+you\\s+can\\s+see',
  'a\\s+lot\\s+of',
  'lots\\s+of',
  'various',
];
const FILLER_RE = new RegExp(`(?<![\\w-])(?:${FILLER.join('|')})(?![\\w-])`, 'gi');

/** D2 */
function fillerHits(text: string): string[] {
  return [...text.matchAll(FILLER_RE)].map((m) => `D2 filler "${m[0]}": cut it`);
}

const ABBREVIATIONS =
  /(?<![\w/])(?:cfg|conf|impl|reqs?|res|resp|fn|func|msgs?|ctx|tmp|btn|approx|w\/o|w\/|b\/c)(?![\w/])/gi;

/** D3 */
function abbreviationHits(text: string): string[] {
  return [...text.matchAll(ABBREVIATIONS)].map(
    (m) => `D3 invented abbreviation "${m[0]}": write the full word`,
  );
}

const ARROWS = /[→←↔⇒⇐⟶⟵]|->|=>|<-/g;

/** D4 */
function arrowHits(text: string): string[] {
  return [...text.matchAll(ARROWS)].map((m) => `D4 arrow "${m[0]}": use a word`);
}

/** D5 */
function emojiHits(text: string): string[] {
  return [...text.matchAll(/\p{Extended_Pictographic}/gu)].map(
    (m) => `D5 emoji "${m[0]}": remove it`,
  );
}

const TERMS: ReadonlyArray<readonly [RegExp, string]> = [
  [/(?<![\w-])repos?(?![\w-])/gi, 'repository'],
  [/(?<![\w-])dirs?(?![\w-])/gi, 'folder'],
  [/(?<![\w-])director(?:y|ies)(?![\w-])/gi, 'folder'],
  [/(?<![\w-])preferences(?![\w-])/gi, 'Settings'],
  [/(?<![\w-])canceled(?![\w-])/gi, 'cancelled'],
  [/(?<![\w-])e\.g\./gi, 'for example'],
  [/(?<![\w-])i\.e\./gi, 'that is'],
  [/(?<![\w-])etc\./gi, 'name the items'],
  [/(?<![\w-])Github(?![\w-])/g, 'GitHub'],
  [/(?<![\w-])MacOS(?![\w-])/g, 'macOS'],
  [/(?<![\w-])Postgres(?![\w-])/g, 'PostgreSQL'],
];
// Colour is the UI spelling (BANNED in scripts/copy/catalogRules.ts); only the US form is flagged.
const COLOR_US = /(?<![\w-])colors?(?![\w-])/gi;

/** D7 */
function termHits(text: string): string[] {
  const out: string[] = [];
  for (const [re, fix] of TERMS) {
    for (const m of text.matchAll(re)) out.push(`D7 term "${m[0]}": use "${fix}"`);
  }
  for (const m of text.matchAll(COLOR_US))
    out.push(`D7 term "${m[0]}": use "${m[0].replace(/o/i, 'ou')}"`);
  return out;
}

/** D8 */
function markerHits(text: string): string[] {
  return [...text.matchAll(/(?<![\w-])(?:TODO|TBD|FIXME|XXX)(?![\w-])/g)].map(
    (m) => `D8 marker "${m[0]}": resolve it`,
  );
}

/** D6: `rows` is every table row (header first) as cell texts. */
function tableHit(rows: readonly (readonly string[])[]): string | undefined {
  const cols = rows[0]?.length ?? 0;
  const body = rows.length - 1;
  if (cols < 3 || body < 3) return `D6 table has ${cols} columns and ${body} body rows: use a list`;
  if (rows.some((r) => r.some((c) => c.trim() === ''))) return 'D6 table has an empty cell';
  return undefined;
}

/** D2 and D1 are off inside asides; every other rule always runs. */
function proseHits(text: string, relaxed: boolean): string[] {
  return [
    ...(relaxed ? [] : longSentences(text)),
    ...(relaxed ? [] : fillerHits(text)),
    ...abbreviationHits(text),
    ...arrowHits(text),
    ...emojiHits(text),
    ...termHits(text),
    ...markerHits(text),
  ];
}

interface Node {
  type: string;
  value?: string;
  name?: string;
  children?: Node[];
  position?: { start: { line: number } };
}

function inline(node: Node): string {
  switch (node.type) {
    case 'text':
      return node.value ?? '';
    case 'inlineCode':
    case 'mdxTextExpression':
      return CODE;
    case 'break':
      return ' ';
    case 'image':
    case 'imageReference':
    case 'html':
    case 'footnoteReference':
      return '';
    default:
      return (node.children ?? []).map(inline).join('');
  }
}

const ASIDES = new Set(['caution', 'danger']);
const PROSE_BLOCKS = new Set(['paragraph', 'heading', 'tableCell']);

function collect(source: string, isMdx: boolean): { yaml?: string; violations: Violation[] } {
  const processor = unified()
    .use(remarkParse)
    .use(remarkFrontmatter, ['yaml'])
    .use(remarkGfm)
    .use(remarkDirective);
  if (isMdx) processor.use(remarkMdx);
  const tree = processor.parse(source) as unknown as Node;
  const violations: Violation[] = [];
  let yaml: string | undefined;

  const emit = (node: Node, messages: string[]) => {
    for (const m of messages) {
      const rule = m.slice(0, 2);
      violations.push({ line: node.position?.start.line ?? 1, rule, message: m.slice(3) });
    }
  };

  const walk = (node: Node, relaxed: boolean) => {
    if (node.type === 'yaml') {
      yaml = node.value;
      return;
    }
    if (['code', 'html', 'definition', 'mdxjsEsm', 'mdxFlowExpression'].includes(node.type)) return;
    if (node.type === 'table') {
      const rows = (node.children ?? []).map((r) =>
        (r.children ?? []).map((c) => inline(c).replaceAll(CODE, 'x')),
      );
      const hit = tableHit(rows);
      if (hit) emit(node, [hit]);
    }
    if (PROSE_BLOCKS.has(node.type)) {
      const text = (node.children ?? []).map(inline).join('');
      const hits = proseHits(text, relaxed);
      emit(node, node.type === 'heading' ? hits.filter((h) => !h.startsWith('D1')) : hits);
      return;
    }
    const inAside = relaxed || (node.type === 'containerDirective' && ASIDES.has(node.name ?? ''));
    for (const child of node.children ?? []) walk(child, inAside);
  };
  walk(tree, false);
  return { yaml, violations };
}

function sourceExists(root: string, path: string): boolean {
  if (!path.includes('*')) return existsSync(join(root, path));
  return new Glob(path).scanSync({ cwd: root }).next().done === false;
}

function frontmatterHits(yaml: string | undefined, source: string, root: string): Violation[] {
  const lineOf = (key: string) =>
    Math.max(1, source.split('\n').findIndex((l) => l.startsWith(`${key}:`)) + 1);
  const v = (key: string, message: string): Violation => ({
    line: lineOf(key),
    rule: 'D9',
    message,
  });
  if (yaml === undefined) return [{ line: 1, rule: 'D9', message: 'missing frontmatter' }];
  const data = (parseYaml(yaml) ?? {}) as Record<string, unknown>;
  const out: Violation[] = [];
  const asProse = (key: string, text: string) => {
    for (const m of proseHits(text, false))
      out.push({ line: lineOf(key), rule: m.slice(0, 2), message: m.slice(3) });
  };
  if (typeof data.title !== 'string' || !data.title) out.push(v('title', 'title missing'));
  else asProse('title', data.title);
  if (typeof data.description !== 'string' || !data.description)
    out.push(v('description', 'description missing'));
  else {
    if (splitSentences(data.description).length !== 1)
      out.push(v('description', 'description is not one sentence'));
    asProse('description', data.description);
  }
  const sources = data.sources;
  if (!Array.isArray(sources) || sources.length === 0) out.push(v('sources', 'sources missing'));
  else
    for (const s of sources)
      if (typeof s !== 'string' || !sourceExists(root, s))
        out.push(v('sources', `source not found: ${String(s)}`));
  return out;
}

/** Checks one docs page; `root` is the repo root that `sources:` paths resolve against. */
export function checkDocument(source: string, isMdx: boolean, root: string): Violation[] {
  const { yaml, violations } = collect(source, isMdx);
  return [...frontmatterHits(yaml, source, root), ...violations].sort((a, b) => a.line - b.line);
}
