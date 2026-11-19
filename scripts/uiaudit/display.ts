import ts from 'typescript';
import { stableKey } from './cluster';
import { loadFiles, type SourceFile } from './files';
import { type ChannelResult, type Cluster, type Site, sortClusters } from './report';
import {
  type ParsedCode,
  parseExpression,
  scriptBlocks,
  templateExpressions,
  textOf,
  visit,
} from './sfc';

/** Formatter homes: display code may live here, so nothing inside is a hit. */
const HOMES = [
  /^packages\/theme\/src\/format\.ts$/,
  /^packages\/theme\/src\/i18n\/number\.ts$/,
  /^packages\/workbench\/src\/behaviour\/format\.ts$/,
  /^packages\/workbench\/src\/shortcuts\//,
  /^packages\/workbench\/src\/components\/Identifier\.vue$/,
];

type Kind = 'date' | 'size' | 'duration' | 'count' | 'id' | 'path' | 'shortcut' | 'missing';

const KIND_HINTS: [Kind, RegExp][] = [
  ['size', /byte|size|\bkb\b|\bmb\b|\bgb\b|1024/i],
  ['duration', /durat|elapsed|\bms\b|milli|second|\bsec\b|latency|took|\bmin(ute)?s?\b/i],
  ['date', /date|time|\bday|month|year|\bago\b|hour|stamp|\bat\b/i],
  ['id', /\bid\b|sha|hash|commit|oid|digest/i],
  ['path', /path|\bdir|folder|file/i],
];

function classify(text: string, fallback: Kind): Kind {
  for (const [kind, re] of KIND_HINTS) if (re.test(text)) return kind;
  return fallback;
}

const DATE_GETTERS = new Set([
  'getHours',
  'getMinutes',
  'getSeconds',
  'getMilliseconds',
  'getFullYear',
  'getMonth',
  'getDate',
  'getDay',
  'getUTCHours',
  'getUTCMinutes',
  'getUTCSeconds',
  'getUTCFullYear',
  'getUTCMonth',
  'getUTCDate',
  'getUTCDay',
]);
const INTL_SKIP = new Set(['Collator', 'Segmenter']);
const SHORTCUT_GLYPH = /[⌘⇧⌥⌃↵⎋⌫⇥]|\b(Ctrl|Cmd|Alt|Shift)\+/;
const ID_SLICE = new Set([7, 8, 12]);
const UNIT = /^ ?(ms|s|m|h|d|B|KB|kB|MB|GB|TB|%)(?![A-Za-z])/;
const SINGLE_UNIT = new Set(['s', 'm', 'h', 'd']);

interface Hit {
  kind: Kind;
  pattern: string;
  line: number;
}

function numberValue(n: ts.Node | undefined): number | undefined {
  return n && ts.isNumericLiteral(n) ? Number(n.text) : undefined;
}

function stringValue(n: ts.Node | undefined): string | undefined {
  return n && (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n)) ? n.text : undefined;
}

interface Found {
  kind: Kind;
  pattern: string;
}

function intlHit(n: ts.Node): Found | undefined {
  if (
    !ts.isPropertyAccessExpression(n) ||
    !ts.isIdentifier(n.expression) ||
    n.expression.text !== 'Intl' ||
    INTL_SKIP.has(n.name.text)
  ) {
    return undefined;
  }
  return {
    kind: /Date|Relative/.test(n.name.text) ? 'date' : 'count',
    pattern: `Intl.${n.name.text}`,
  };
}

function methodHit(n: ts.CallExpression, sf: ts.SourceFile): Found | undefined {
  if (!ts.isPropertyAccessExpression(n.expression)) return undefined;
  const method = n.expression.name.text;
  const whole = textOf(n, sf);
  switch (method) {
    case 'toLocaleDateString':
    case 'toLocaleTimeString':
      return { kind: 'date', pattern: method };
    case 'toLocaleString':
      return {
        kind: classify(textOf(n.expression.expression, sf), 'count') === 'date' ? 'date' : 'count',
        pattern: method,
      };
    case 'toFixed':
      return { kind: classify(whole, 'count'), pattern: method };
    case 'padStart': {
      const kind = classify(whole, 'date');
      return { kind: kind === 'count' ? 'date' : kind, pattern: method };
    }
    case 'slice':
    case 'substring': {
      const end = numberValue(n.arguments[1]);
      if (numberValue(n.arguments[0]) !== 0 || end === undefined || !ID_SLICE.has(end)) {
        return undefined;
      }
      return { kind: 'id', pattern: `slice(0,${end})` };
    }
    case 'split': {
      const sep = stringValue(n.arguments[0]);
      if (sep !== '/' && sep !== '\\') return undefined;
      return { kind: 'path', pattern: `split('${sep}')` };
    }
    default:
      return DATE_GETTERS.has(method) ? { kind: 'date', pattern: `Date#${method}` } : undefined;
  }
}

function byteMathHit(n: ts.Node): Found | undefined {
  if (!ts.isBinaryExpression(n)) return undefined;
  const op = n.operatorToken.kind;
  const mul = op === ts.SyntaxKind.SlashToken || op === ts.SyntaxKind.AsteriskToken;
  if (mul && (numberValue(n.right) === 1024 || numberValue(n.left) === 1024)) {
    return { kind: 'size', pattern: 'byte math 1024' };
  }
  return undefined;
}

function unitHits(n: ts.Node, sf: ts.SourceFile): Found[] {
  const out: Found[] = [];
  if (!ts.isTemplateExpression(n)) return out;
  for (const span of n.templateSpans) {
    const unit = UNIT.exec(span.literal.text)?.[1];
    if (!unit) continue;
    if (SINGLE_UNIT.has(unit) && classify(textOf(span.expression, sf), 'count') !== 'duration') {
      continue;
    }
    const kind: Kind = unit === '%' ? 'count' : /B$/.test(unit) ? 'size' : 'duration';
    out.push({ kind, pattern: `unit suffix \`${unit}\`` });
  }
  return out;
}

function literalHits(n: ts.Node): Found[] {
  const lit = stringValue(n);
  if (lit === undefined || !ts.isStringLiteralLike(n)) return [];
  const out: Found[] = [];
  if (SHORTCUT_GLYPH.test(lit)) out.push({ kind: 'shortcut', pattern: 'shortcut glyph string' });
  if (/^[—–]$/.test(lit) || (lit === '-' && isFallbackBranch(n))) {
    out.push({ kind: 'missing', pattern: `placeholder '${lit}'` });
  }
  return out;
}

function hitsIn(p: ParsedCode): Hit[] {
  const out: Hit[] = [];
  visit(p.sf, (n) => {
    const found: (Found | undefined)[] = [
      intlHit(n),
      ts.isCallExpression(n) ? methodHit(n, p.sf) : undefined,
      byteMathHit(n),
      ...unitHits(n, p.sf),
      ...literalHits(n),
    ];
    for (const f of found) if (f) out.push({ ...f, line: p.lineOf(n) });
  });
  return out;
}

function isFallbackBranch(n: ts.Node): boolean {
  const parent = n.parent;
  if (ts.isBinaryExpression(parent)) {
    const k = parent.operatorToken.kind;
    return (
      (k === ts.SyntaxKind.QuestionQuestionToken || k === ts.SyntaxKind.BarBarToken) &&
      parent.right === n
    );
  }
  return ts.isConditionalExpression(parent) && (parent.whenTrue === n || parent.whenFalse === n);
}

function fileHits(f: SourceFile): Hit[] {
  const out: Hit[] = [];
  for (const b of scriptBlocks(f)) out.push(...hitsIn(b));
  if (f.ext === 'vue') {
    for (const e of templateExpressions(f)) out.push(...hitsIn(parseExpression(e)));
  }
  return out;
}

export function runDisplay(): ChannelResult {
  const groups = new Map<string, { kind: Kind; pattern: string; sites: Site[] }>();
  for (const f of loadFiles()) {
    if (HOMES.some((r) => r.test(f.path))) continue;
    for (const h of fileHits(f)) {
      const key = `${h.kind}|${h.pattern}`;
      const g = groups.get(key) ?? { kind: h.kind, pattern: h.pattern, sites: [] };
      g.sites.push({ file: f.path, line: h.line, module: f.module });
      groups.set(key, g);
    }
  }
  const clusters: Cluster[] = [...groups].map(([key, g]) => ({
    key: stableKey(`display|${key}`),
    title: `${g.kind}: ${g.pattern}`,
    size: g.sites.length,
    sizeUnit: 'sites',
    sites: g.sites,
  }));
  return { id: 'c5', title: 'Display drift', clusters: sortClusters(clusters) };
}
