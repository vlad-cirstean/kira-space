import {
  type AttributeNode,
  type DirectiveNode,
  type ElementNode,
  NodeTypes,
  type RootNode,
  type TemplateChildNode,
} from '@vue/compiler-core';
import ts from 'typescript';
import type { SourceFile } from './files';

type WalkNode = RootNode | TemplateChildNode | { type: NodeTypes.IF_BRANCH; children: unknown[] };

/** Child elements of a node, looking through `v-if` and `v-for` wrappers. */
export function childElements(node: { children?: unknown[] }): ElementNode[] {
  const out: ElementNode[] = [];
  const visit = (n: WalkNode): void => {
    switch (n.type) {
      case NodeTypes.ELEMENT:
        out.push(n);
        break;
      case NodeTypes.FOR:
      case NodeTypes.IF_BRANCH:
        for (const c of n.children as WalkNode[]) visit(c);
        break;
      case NodeTypes.IF:
        for (const b of n.branches) visit(b as unknown as WalkNode);
        break;
      default:
    }
  };
  for (const c of (node.children ?? []) as WalkNode[]) visit(c);
  return out;
}

export function templateRoot(file: SourceFile): RootNode | null {
  const ast = file.descriptor?.template?.ast;
  return ast ?? null;
}

export function walkElements(
  file: SourceFile,
  fn: (el: ElementNode, parent: ElementNode | null) => void,
): void {
  const root = templateRoot(file);
  if (!root) return;
  const go = (n: { children?: unknown[] }, parent: ElementNode | null): void => {
    for (const el of childElements(n)) {
      fn(el, parent);
      go(el, el);
    }
  };
  go(root, null);
}

export function staticAttr(el: ElementNode, name: string): AttributeNode | undefined {
  return el.props.find(
    (p): p is AttributeNode => p.type === NodeTypes.ATTRIBUTE && p.name === name,
  );
}

export function directives(el: ElementNode): DirectiveNode[] {
  return el.props.filter((p): p is DirectiveNode => p.type === NodeTypes.DIRECTIVE);
}

export function directiveName(d: DirectiveNode): string {
  const arg = d.arg && 'content' in d.arg ? d.arg.content : '';
  return arg ? `${d.name}:${arg}` : d.name;
}

/** Every expression of a template: interpolations and directive values, with start line. */
export function templateExpressions(
  file: SourceFile,
): { code: string; line: number; raw: boolean; directive?: string; el?: ElementNode }[] {
  const out: { code: string; line: number; raw: boolean; directive?: string; el?: ElementNode }[] =
    [];
  const root = templateRoot(file);
  if (!root) return out;
  const visit = (n: WalkNode): void => {
    switch (n.type) {
      case NodeTypes.INTERPOLATION:
        if ('content' in n.content) {
          out.push({ code: n.content.content, line: n.content.loc.start.line, raw: false });
        }
        break;
      case NodeTypes.ELEMENT:
        for (const d of directives(n)) {
          if (d.exp && 'content' in d.exp && d.exp.content.trim()) {
            const raw = d.name === 'on' || d.name === 'for' || d.name === 'slot';
            out.push({
              code: d.exp.content,
              line: d.exp.loc.start.line,
              raw,
              directive: directiveName(d),
              el: n,
            });
          }
        }
        for (const c of n.children as WalkNode[]) visit(c);
        break;
      case NodeTypes.FOR:
      case NodeTypes.IF_BRANCH:
        for (const c of n.children as WalkNode[]) visit(c);
        break;
      case NodeTypes.IF:
        for (const b of n.branches) visit(b as unknown as WalkNode);
        break;
      default:
    }
  };
  for (const c of root.children) visit(c);
  return out;
}

export interface ParsedCode {
  sf: ts.SourceFile;
  /** File line of a node's first character. */
  lineOf: (node: ts.Node) => number;
}

/** A script block, as TS AST with file-relative line numbers. */
export function scriptBlocks(file: SourceFile): ParsedCode[] {
  if (file.ext === 'ts') {
    const sf = ts.createSourceFile(file.path, file.source, ts.ScriptTarget.Latest, true);
    return [{ sf, lineOf: (n) => sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1 }];
  }
  const d = file.descriptor;
  if (!d) return [];
  const blocks: ParsedCode[] = [];
  for (const b of [d.script, d.scriptSetup]) {
    if (!b) continue;
    const sf = ts.createSourceFile(`${file.path}.ts`, b.content, ts.ScriptTarget.Latest, true);
    const base = b.loc.start.line;
    blocks.push({
      sf,
      lineOf: (n) => base + sf.getLineAndCharacterOfPosition(n.getStart(sf)).line,
    });
  }
  return blocks;
}

/** A template expression as TS AST; wrapped in parens unless it is a statement list. */
export function parseExpression(e: { code: string; line: number; raw: boolean }): ParsedCode {
  const text = e.raw ? e.code : `(${e.code})`;
  const sf = ts.createSourceFile('expr.ts', text, ts.ScriptTarget.Latest, true);
  return {
    sf,
    lineOf: (n) => e.line + sf.getLineAndCharacterOfPosition(n.getStart(sf)).line,
  };
}

export function visit(node: ts.Node, fn: (n: ts.Node) => void): void {
  fn(node);
  ts.forEachChild(node, (c) => visit(c, fn));
}

/** Source text of a callee or receiver, trimmed to one line. */
export function textOf(node: ts.Node, sf: ts.SourceFile): string {
  return node.getText(sf).replace(/\s+/g, ' ');
}

/** The script text a file's code lives in (all blocks), for coarse presence checks. */
export function scriptText(file: SourceFile): string {
  if (file.ext === 'ts') return file.source;
  const d = file.descriptor;
  return `${d?.script?.content ?? ''}\n${d?.scriptSetup?.content ?? ''}`;
}
