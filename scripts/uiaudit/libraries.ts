import ts from 'typescript';
import { stableKey } from './cluster';
import { loadFiles, type SourceFile } from './files';
import { type ChannelResult, type Cluster, type Site, sortClusters } from './report';
import {
  type ParsedCode,
  parseExpression,
  scriptBlocks,
  scriptText,
  staticAttr,
  templateExpressions,
  textOf,
  visit,
  walkElements,
} from './sfc';

interface Hit {
  pattern: string;
  line: number;
}

const OBSERVERS = new Set(['ResizeObserver', 'IntersectionObserver', 'MutationObserver']);
const STORAGE = new Set(['localStorage', 'sessionStorage']);
const STATE_REF = /^(is)?(loading|busy|pending|saving|submitting|error|err|loadError)$/i;
const ROLES = new Set(['menu', 'dialog', 'listbox', 'tab']);

function callPattern(
  n: ts.CallExpression,
  sf: ts.SourceFile,
  hasClear: boolean,
): string | undefined {
  const callee = textOf(n.expression, sf);
  const last = callee.split('.').pop() ?? '';
  if (last === 'addEventListener' || last === 'removeEventListener') {
    const ev = n.arguments[0];
    return `${last}('${ev && ts.isStringLiteralLike(ev) ? ev.text : '*'}')`;
  }
  if (callee === 'setInterval' || callee === 'window.setInterval') return 'setInterval poll';
  if ((callee === 'setTimeout' || callee === 'window.setTimeout') && hasClear) {
    return 'setTimeout + clearTimeout debounce';
  }
  return undefined;
}

function globalPattern(n: ts.Node, sf: ts.SourceFile): string | undefined {
  if (ts.isNewExpression(n) && ts.isIdentifier(n.expression) && OBSERVERS.has(n.expression.text)) {
    return `new ${n.expression.text}`;
  }
  if (ts.isIdentifier(n) && STORAGE.has(n.text) && !ts.isPropertyAccessExpression(n.parent)) {
    return n.text;
  }
  if (ts.isPropertyAccessExpression(n)) {
    if (ts.isIdentifier(n.expression) && STORAGE.has(n.expression.text)) return n.expression.text;
    if (textOf(n, sf) === 'navigator.clipboard') return 'navigator.clipboard';
  }
  return undefined;
}

function moduleStateHits(p: ParsedCode): Hit[] {
  const out: Hit[] = [];
  for (const st of p.sf.statements) {
    if (!ts.isVariableStatement(st)) continue;
    for (const d of st.declarationList.declarations) {
      const init = d.initializer;
      if (
        init &&
        ts.isCallExpression(init) &&
        ts.isIdentifier(init.expression) &&
        /^(ref|reactive|shallowRef|shallowReactive)$/.test(init.expression.text)
      ) {
        out.push({ pattern: `module-level ${init.expression.text}`, line: p.lineOf(d) });
      }
    }
  }
  return out;
}

function codeHits(p: ParsedCode, file: SourceFile): Hit[] {
  const out: Hit[] = [];
  const text = file.ext === 'vue' ? scriptText(file) : file.source;
  const hasClear = /\bclearTimeout\b/.test(text);
  visit(p.sf, (n) => {
    const pattern =
      (ts.isCallExpression(n) ? callPattern(n, p.sf, hasClear) : undefined) ??
      globalPattern(n, p.sf);
    if (pattern) out.push({ pattern, line: p.lineOf(n) });
  });
  if (file.ext === 'ts' && !/\bdefineStore\(/.test(text)) out.push(...moduleStateHits(p));
  return out;
}

/** Loading or error ref in a script that awaits calls: a hand-rolled query state. */
function queryStateHits(file: SourceFile): Hit[] {
  const out: Hit[] = [];
  for (const b of scriptBlocks(file)) {
    let awaits = false;
    const refs: ts.Node[] = [];
    visit(b.sf, (n) => {
      if (ts.isAwaitExpression(n)) awaits = true;
      if (
        ts.isVariableDeclaration(n) &&
        ts.isIdentifier(n.name) &&
        STATE_REF.test(n.name.text) &&
        n.initializer &&
        /^(ref|shallowRef)\b/.test(n.initializer.getText(b.sf))
      ) {
        refs.push(n);
      }
    });
    if (awaits)
      for (const r of refs)
        out.push({ pattern: 'loading/error ref around awaited call', line: b.lineOf(r) });
  }
  return out;
}

function templateHits(file: SourceFile): Hit[] {
  const out: Hit[] = [];
  walkElements(file, (el) => {
    const line = el.loc.start.line;
    if (el.tag === 'select') out.push({ pattern: 'raw <select>', line });
    if (el.tag === 'dialog') out.push({ pattern: 'raw <dialog>', line });
    if (el.tag === 'input') {
      const type = staticAttr(el, 'type')?.value?.content;
      if (type === 'checkbox' || type === 'radio')
        out.push({ pattern: `raw <input type=${type}>`, line });
    }
    const role = staticAttr(el, 'role')?.value?.content;
    if (role && ROLES.has(role) && el.tag === el.tag.toLowerCase() && el.tag.indexOf('-') < 0) {
      out.push({ pattern: `role=${role} on <${el.tag}>`, line });
    }
    const tokens = (staticAttr(el, 'class')?.value?.content ?? '').split(/\s+/);
    if (
      tokens.includes('absolute') &&
      tokens.some((t) => /^z-/.test(t)) &&
      tokens.some((t) => /^shadow/.test(t)) &&
      tokens.some((t) => t === 'border' || /^bg-/.test(t))
    ) {
      out.push({ pattern: 'absolutely positioned popup', line });
    }
  });
  return out;
}

/** Comment run directly above a line, or trailing on it: the documented reason, if any. */
function commentFor(file: SourceFile, line: number): string | undefined {
  const lines = file.source.split('\n');
  const parts: string[] = [];
  for (let i = line - 2; i >= 0 && i >= line - 8; i--) {
    const t = lines[i].trim();
    if (/^(\/\/|\/\*|\*|<!--)/.test(t) || /(-->|\*\/)$/.test(t)) {
      parts.unshift(
        t
          .replace(/^(\/\/+|\/\*+|\*+|<!--)\s?/, '')
          .replace(/(-->|\*\/)$/, '')
          .trim(),
      );
    } else break;
  }
  const trailing = /(?:\/\/|<!--)\s*(.+?)(?:-->)?\s*$/.exec(lines[line - 1] ?? '');
  if (trailing && !/['"`]\s*\/\//.test(trailing[0])) parts.push(trailing[1]);
  const joined = parts.join(' ').trim();
  return joined ? joined.slice(0, 220) : undefined;
}

export function runLibraries(): ChannelResult {
  const groups = new Map<string, Site[]>();
  for (const f of loadFiles()) {
    const hits: Hit[] = [];
    for (const b of scriptBlocks(f)) hits.push(...codeHits(b, f));
    if (f.ext === 'vue') {
      for (const e of templateExpressions(f)) hits.push(...codeHits(parseExpression(e), f));
      hits.push(...templateHits(f));
    }
    hits.push(...queryStateHits(f));
    for (const h of hits) {
      const comment = commentFor(f, h.line);
      const list = groups.get(h.pattern) ?? [];
      list.push({
        file: f.path,
        line: h.line,
        module: f.module,
        note: comment ? `[comment: ${comment}]` : undefined,
      });
      groups.set(h.pattern, list);
    }
  }
  const clusters: Cluster[] = [...groups].map(([pattern, sites]) => ({
    key: stableKey(`library|${pattern}`),
    title: pattern,
    size: sites.length,
    sizeUnit: 'sites',
    sites,
  }));
  return { id: 'c7', title: 'Library near-miss', clusters: sortClusters(clusters) };
}
