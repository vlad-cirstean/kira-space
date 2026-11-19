import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import type { ElementNode } from '@vue/compiler-core';
import { NodeTypes } from '@vue/compiler-core';
import ts from 'typescript';
import { ICON } from '../../packages/theme/src/icons';
import { stableKey } from './cluster';
import { loadFiles, REPO_ROOT, type SourceFile } from './files';
import { type ChannelResult, type Cluster, type Site, sortClusters } from './report';
import { childElements, directives, templateRoot, visit } from './sfc';

const GENERIC = new Set([
  'label',
  'title',
  'tooltip',
  'hint',
  'aria',
  'arialabel',
  'text',
  'description',
  'placeholder',
  'button',
  'action',
  'actions',
  'name',
  'empty',
  'heading',
  'header',
]);
const T_KEY = /\bt\(\s*['"`]([\w.-]+)['"`]/;
const ICON_GLYPHS = new Set<string>(Object.values(ICON));
const ICON_BY_GLYPH = new Map<string, string>();
for (const [concept, glyph] of Object.entries(ICON)) {
  if (!ICON_BY_GLYPH.has(glyph)) ICON_BY_GLYPH.set(glyph, concept);
}

interface IconUse {
  glyph: string;
  /** True when spelled as a literal glyph name instead of `ICON.<concept>`. */
  literal: boolean;
  concept: string | undefined;
  site: Site;
}

/** Concept of an icon: the catalog key next to it, generic trailing segments dropped. */
function conceptFromKey(key: string): string {
  const parts = key.split('.').filter((p) => !GENERIC.has(p.toLowerCase()));
  return (parts[parts.length - 1] ?? key.split('.').pop() ?? key).toLowerCase();
}

function keyIn(el: ElementNode): string | undefined {
  for (const d of directives(el)) {
    if (d.exp && 'content' in d.exp) {
      const m = T_KEY.exec(d.exp.content);
      if (m) return m[1];
    }
  }
  for (const c of el.children) {
    if (c.type === NodeTypes.INTERPOLATION && 'content' in c.content) {
      const m = T_KEY.exec(c.content.content);
      if (m) return m[1];
    }
  }
  return undefined;
}

function resolveGlyph(expr: string): { glyph: string; literal: boolean } | undefined {
  const t = expr.trim();
  const ref = /^ICON\.(\w+)$/.exec(t);
  if (ref) {
    const g = (ICON as Record<string, string>)[ref[1]];
    return g ? { glyph: g, literal: false } : undefined;
  }
  const lit = /^['"]([\w-]+)['"]$/.exec(t);
  return lit ? { glyph: lit[1], literal: true } : undefined;
}

interface IconAttr {
  value: string;
  line: number;
  literalAttr: boolean;
}

/** `icon` props, and `name` on CodiconIcon, static or bound. */
function iconAttrs(el: ElementNode): IconAttr[] {
  const isIconTag = el.tag === 'CodiconIcon';
  const wanted = (name: string): boolean => name === 'icon' || (isIconTag && name === 'name');
  const out: IconAttr[] = [];
  for (const p of el.props) {
    if (p.type === NodeTypes.ATTRIBUTE && wanted(p.name) && p.value) {
      out.push({ value: p.value.content, line: p.loc.start.line, literalAttr: true });
    } else if (
      p.type === NodeTypes.DIRECTIVE &&
      p.name === 'bind' &&
      p.arg &&
      'content' in p.arg &&
      wanted(p.arg.content) &&
      p.exp &&
      'content' in p.exp
    ) {
      out.push({ value: p.exp.content, line: p.loc.start.line, literalAttr: false });
    }
  }
  return out;
}

/** Catalog key of the label on the element or its two nearest ancestors. */
function nearbyKey(el: ElementNode, up: ElementNode[]): string | undefined {
  for (const cand of [el, ...[...up].reverse().slice(0, 2)]) {
    const key = keyIn(cand);
    if (key) return key;
  }
  return undefined;
}

function templateIconUses(file: SourceFile): IconUse[] {
  const out: IconUse[] = [];
  const root = templateRoot(file);
  if (!root) return out;
  const go = (el: ElementNode, up: ElementNode[]): void => {
    for (const nm of iconAttrs(el)) {
      const g = nm.literalAttr ? { glyph: nm.value, literal: true } : resolveGlyph(nm.value);
      if (!g || !/^[\w-]+$/.test(g.glyph)) continue;
      const key = nearbyKey(el, up);
      out.push({
        glyph: g.glyph,
        literal: g.literal,
        concept: key ? conceptFromKey(key) : undefined,
        site: { file: file.path, line: nm.line, module: file.module },
      });
    }
    for (const c of childElements(el)) go(c, [...up, el]);
  };
  for (const c of childElements(root)) go(c, []);
  return out;
}

function scriptIconUses(file: SourceFile): IconUse[] {
  const out: IconUse[] = [];
  if (file.ext !== 'ts') return out;
  const sf = ts.createSourceFile(file.path, file.source, ts.ScriptTarget.Latest, true);
  visit(sf, (n) => {
    if (!ts.isObjectLiteralExpression(n)) return;
    let icon: ts.PropertyAssignment | undefined;
    let key: string | undefined;
    for (const p of n.properties) {
      if (!ts.isPropertyAssignment(p) || !ts.isIdentifier(p.name)) continue;
      if (p.name.text === 'icon') icon = p;
      const m = /^(label|title|tooltip)$/.test(p.name.text)
        ? T_KEY.exec(p.initializer.getText(sf))
        : null;
      if (m) key = m[1];
    }
    if (!icon) return;
    const g = resolveGlyph(icon.initializer.getText(sf));
    if (!g) return;
    out.push({
      glyph: g.glyph,
      literal: g.literal,
      concept: key ? conceptFromKey(key) : undefined,
      site: {
        file: file.path,
        line: sf.getLineAndCharacterOfPosition(icon.getStart(sf)).line + 1,
        module: file.module,
      },
    });
  });
  return out;
}

function iconClusters(uses: IconUse[]): Cluster[] {
  const out: Cluster[] = [];
  const byConcept = new Map<string, IconUse[]>();
  const byGlyph = new Map<string, IconUse[]>();
  for (const u of uses) {
    if (u.concept) {
      const l = byConcept.get(u.concept) ?? [];
      l.push(u);
      byConcept.set(u.concept, l);
      const g = byGlyph.get(u.glyph) ?? [];
      g.push(u);
      byGlyph.set(u.glyph, g);
    }
  }
  for (const [concept, list] of byConcept) {
    const glyphs = [...new Set(list.map((u) => u.glyph))].sort();
    if (glyphs.length < 2) continue;
    out.push({
      key: stableKey(`concept-icons|${concept}`),
      title: `one concept, ${glyphs.length} icons: ${concept}`,
      size: glyphs.length,
      sizeUnit: 'icons',
      sites: list.map((u) => ({ ...u.site, note: u.glyph })),
      note: `Icons: ${glyphs.join(', ')}.`,
    });
  }
  for (const [glyph, list] of byGlyph) {
    if (ICON_GLYPHS.has(glyph)) continue;
    const concepts = [...new Set(list.map((u) => u.concept))].sort();
    if (concepts.length < 3) continue;
    out.push({
      key: stableKey(`icon-concepts|${glyph}`),
      title: `one icon, ${concepts.length} concepts: ${glyph}`,
      size: concepts.length,
      sizeUnit: 'concepts',
      sites: list.map((u) => ({ ...u.site, note: u.concept })),
      note: `Concepts: ${concepts.join(', ')}.`,
    });
  }
  const bypass = new Map<string, IconUse[]>();
  for (const u of uses) {
    if (!u.literal || !ICON_BY_GLYPH.has(u.glyph)) continue;
    const l = bypass.get(u.glyph) ?? [];
    l.push(u);
    bypass.set(u.glyph, l);
  }
  for (const [glyph, list] of bypass) {
    out.push({
      key: stableKey(`icon-bypass|${glyph}`),
      title: `literal icon \`${glyph}\` bypasses ICON.${ICON_BY_GLYPH.get(glyph)}`,
      size: list.length,
      sizeUnit: 'sites',
      sites: list.map((u) => u.site),
    });
  }
  return out;
}

function flatten(o: unknown, prefix: string, out: Map<string, string>): void {
  if (typeof o === 'string') {
    out.set(prefix, o);
  } else if (o && typeof o === 'object') {
    for (const [k, v] of Object.entries(o)) flatten(v, prefix ? `${prefix}.${k}` : k, out);
  }
}

const CATALOG = 'packages/theme/src/i18n/en.json';
const VERBS = [
  'Remove',
  'Delete',
  'Discard',
  'Clear',
  'Erase',
  'Drop',
  'Forget',
  'Reset',
  'Revert',
];

function catalogClusters(): Cluster[] {
  const flat = new Map<string, string>();
  flatten(JSON.parse(readFileSync(join(REPO_ROOT, CATALOG), 'utf8')), '', flat);
  const site = (key: string): Site => ({
    file: CATALOG,
    line: 1,
    module: `catalog/${key.split('.')[0]}`,
    note: key,
  });
  const out: Cluster[] = [];
  const byText = new Map<string, string[]>();
  const byNoun = new Map<string, { verb: string; key: string }[]>();
  for (const [key, text] of flat) {
    if (key.startsWith('common.')) continue;
    const folded = text.trim().toLowerCase();
    if (folded.length >= 3) {
      const l = byText.get(folded) ?? [];
      l.push(key);
      byText.set(folded, l);
    }
    const m = new RegExp(`^(${VERBS.join('|')}) (.{3,40})$`).exec(text.trim());
    if (m && !/[{}.]/.test(m[2])) {
      const noun = m[2].toLowerCase();
      const l = byNoun.get(noun) ?? [];
      l.push({ verb: m[1], key });
      byNoun.set(noun, l);
    }
  }
  for (const [text, keys] of byText) {
    if (keys.length < 2) continue;
    out.push({
      key: stableKey(`catalog-dup|${text}`),
      title: `English under ${keys.length} keys: "${text}"`,
      size: keys.length,
      sizeUnit: 'keys',
      sites: keys.map(site),
    });
  }
  for (const [noun, list] of byNoun) {
    const verbs = [...new Set(list.map((e) => e.verb))].sort();
    if (verbs.length < 2) continue;
    out.push({
      key: stableKey(`catalog-verbs|${noun}`),
      title: `one action, ${verbs.length} verbs: ${verbs.join(' / ')} ${noun}`,
      size: verbs.length,
      sizeUnit: 'verbs',
      sites: list.map((e) => ({ ...site(e.key), note: `${e.key} (${e.verb})` })),
    });
  }
  return out;
}

export function runVocabulary(): ChannelResult {
  const uses: IconUse[] = [];
  for (const f of loadFiles()) uses.push(...templateIconUses(f), ...scriptIconUses(f));
  return {
    id: 'c6',
    title: 'Vocabulary',
    clusters: sortClusters([...iconClusters(uses), ...catalogClusters()]),
  };
}
