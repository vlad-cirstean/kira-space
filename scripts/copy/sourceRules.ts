/**
 * P265 F3: source text guard rules U64-U71 and U208 (git show bd0c224688d9:docs/v2.2/plans/P265-text-catalog-i18n.md section 3.2).
 * AST-based: `.vue` through `@vue/compiler-sfc`, expressions and TypeScript through `@babel/parser`.
 * Pure: `scanSource` takes the file text and the catalog's placeholder table, returns hits.
 */
import { parse as parseBabel, parseExpression } from '@babel/parser';
import {
  type ElementNode,
  type ExpressionNode,
  NodeTypes,
  type RootNode,
  type TemplateChildNode,
} from '@vue/compiler-core';
import { parse as parseSFC } from '@vue/compiler-sfc';

export const SOURCE_RULES = [
  'U64',
  'U65',
  'U66',
  'U67',
  'U68',
  'U69',
  'U70',
  'U71',
  'U208',
] as const;
export type SourceRule = (typeof SOURCE_RULES)[number];

export interface Hit {
  file: string;
  line: number;
  rule: SourceRule;
  message: string;
}

export interface MessageShape {
  placeholders: ReadonlySet<string>;
  plural: boolean;
}

export interface ScanContext {
  messages: ReadonlyMap<string, MessageShape>;
}

interface Node {
  type: string;
  loc?: { start: { line: number } };
  [key: string]: unknown;
}

const SKIP_KEYS = new Set([
  'loc',
  'start',
  'end',
  'extra',
  'leadingComments',
  'trailingComments',
  'innerComments',
  'typeAnnotation',
  'typeParameters',
  'typeArguments',
  'returnType',
  'errors',
  'tokens',
]);
const TS_EXPRESSION_WRAPPERS = new Set([
  'TSAsExpression',
  'TSSatisfiesExpression',
  'TSNonNullExpression',
  'TSTypeAssertion',
]);
const FUNCTION_SCOPES = new Set([
  'FunctionDeclaration',
  'FunctionExpression',
  'ArrowFunctionExpression',
  'ObjectMethod',
  'ClassMethod',
  'ClassPrivateMethod',
  'ClassProperty',
  'ClassPrivateProperty',
  'StaticBlock',
]);
const TEXT_ATTRS = new Set([
  'title',
  'placeholder',
  'aria-label',
  'aria-description',
  'alt',
  'label',
  'description',
  'message',
  'hint',
  'tooltip',
  'heading',
  'text',
  'subtitle',
  'caption',
  'legend',
]);
const TEXT_ATTR_SUFFIX = /-(?:label|text|title|message|hint|placeholder)$/;
const TEXT_PROPS = new Set([
  'label',
  'title',
  'message',
  'description',
  'placeholder',
  'confirmLabel',
  'cancelLabel',
  'tooltip',
  'hint',
  'text',
  'detail',
  'heading',
  'emptyText',
  'ariaLabel',
  'subtitle',
  'caption',
  'lede',
  'body',
  'reason',
  'whatCopied',
]);
const TEXT_SINKS = new Set([
  'announce',
  'announceApp',
  'copyAnnounced',
  'reportAsyncError',
  'confirm',
]);
const EQUALITY_OPS = new Set(['===', '!==', '==', '!=']);
const I18N_TAGS = new Set(['i18n-t', 'I18nT', 'Translation', 'translation']);
const CATALOG_TEXT_TAGS = new Set(['CatalogText', 'catalog-text']);

function isNode(v: unknown): v is Node {
  return v !== null && typeof v === 'object' && typeof (v as Node).type === 'string';
}

type Visit = (node: Node, parent: Node | null, ancestors: readonly Node[]) => boolean | undefined;

function skipChild(node: Node, key: string, isRoot: boolean): boolean {
  if (SKIP_KEYS.has(key) || (isRoot && key === 'comments')) return true;
  if (node.computed !== true) {
    if (node.type === 'ObjectProperty' && key === 'key') return true;
    if (node.type === 'MemberExpression' && key === 'property') return true;
  }
  return (
    (node.type === 'ImportDeclaration' || node.type === 'ExportNamedDeclaration') &&
    key === 'source'
  );
}

function walk(root: unknown, visit: Visit): void {
  const ancestors: Node[] = [];
  const go = (node: unknown, parent: Node | null): void => {
    if (Array.isArray(node)) {
      for (const n of node) go(n, parent);
      return;
    }
    if (!isNode(node)) return;
    if (node.type.startsWith('TS') && !TS_EXPRESSION_WRAPPERS.has(node.type)) return;
    if (visit(node, parent, ancestors) === false) return;
    ancestors.push(node);
    for (const [k, v] of Object.entries(node)) {
      if (v !== null && typeof v === 'object' && !skipChild(node, k, parent === null)) go(v, node);
    }
    ancestors.pop();
  };
  go(root, null);
}

function str(n: unknown): string | undefined {
  return typeof n === 'string' ? n : undefined;
}

function calleeName(call: Node): string | undefined {
  const callee = call.callee as Node;
  if (callee.type === 'Identifier') return str(callee.name);
  if (callee.type === 'MemberExpression' && callee.computed !== true) {
    return str((callee.property as Node).name);
  }
  return undefined;
}

function isIdentCall(n: unknown, name: string): n is Node {
  if (!isNode(n) || n.type !== 'CallExpression') return false;
  const callee = n.callee as Node;
  return callee.type === 'Identifier' && callee.name === name;
}

/** Cooked text of each static piece of a string or template literal. */
function pieces(n: Node): string[] {
  if (n.type === 'StringLiteral') return [String(n.value)];
  if (n.type === 'TemplateLiteral') {
    return (n.quasis as Node[]).map((q) => str((q.value as { cooked?: string }).cooked) ?? '');
  }
  return [];
}

/** String and template literals that an expression can evaluate to directly. */
function terminalLiterals(n: unknown): Node[] {
  if (!isNode(n)) return [];
  switch (n.type) {
    case 'StringLiteral':
    case 'TemplateLiteral':
      return [n];
    case 'ConditionalExpression':
      return [...terminalLiterals(n.consequent), ...terminalLiterals(n.alternate)];
    case 'LogicalExpression':
      return [...terminalLiterals(n.left), ...terminalLiterals(n.right)];
    case 'BinaryExpression':
      return n.operator === '+' ? [...terminalLiterals(n.left), ...terminalLiterals(n.right)] : [];
    case 'ParenthesizedExpression':
    case 'TSAsExpression':
    case 'TSSatisfiesExpression':
    case 'TSNonNullExpression':
    case 'TSTypeAssertion':
      return terminalLiterals(n.expression);
    default:
      return [];
  }
}

function unwrap(n: unknown): unknown {
  let cur = n;
  while (
    isNode(cur) &&
    (cur.type === 'ParenthesizedExpression' || TS_EXPRESSION_WRAPPERS.has(cur.type))
  ) {
    cur = cur.expression;
  }
  return cur;
}

/** A `t(` call, or an expression that can evaluate to one (conditional branch, logical side). */
function tResult(n: unknown): boolean {
  const e = unwrap(n);
  if (!isNode(e)) return false;
  if (e.type === 'ConditionalExpression') return tResult(e.consequent) || tResult(e.alternate);
  if (e.type === 'LogicalExpression') return tResult(e.left) || tResult(e.right);
  return isIdentCall(e, 't');
}

/** Whitespace around exactly one direct `t(` call is layout, not composition. */
function isPadding(tpl: Node): boolean {
  const exprs = tpl.expressions as unknown[];
  return (
    exprs.length === 1 &&
    isIdentCall(unwrap(exprs[0]), 't') &&
    pieces(tpl).every((p) => p.trim() === '')
  );
}

function composesT(n: unknown): boolean {
  const e = unwrap(n);
  if (!isNode(e)) return false;
  if (tResult(e)) return true;
  return terminalLiterals(e).some((l) => l.type === 'TemplateLiteral' && isComposedTemplate(l));
}

function isComposedTemplate(tpl: Node): boolean {
  return (tpl.expressions as unknown[]).some(composesT) && !isPadding(tpl);
}

/** An array literal (or a spread of one) with a t() result in it. */
function arrayHasT(n: unknown): boolean {
  const e = unwrap(n);
  if (!isNode(e)) return false;
  if (e.type === 'ArrayExpression') {
    return (e.elements as unknown[]).some((el) =>
      isNode(el) && el.type === 'SpreadElement' ? arrayHasT(el.argument) : tResult(el),
    );
  }
  if (e.type === 'ConditionalExpression') return arrayHasT(e.consequent) || arrayHasT(e.alternate);
  if (e.type === 'LogicalExpression') return arrayHasT(e.left) || arrayHasT(e.right);
  return false;
}

/** Identifiers a script block fills with t() results: `const p = [t(..)]` or `p.push(t(..))`. */
function collectTArrays(ast: unknown): Set<string> {
  const names = new Set<string>();
  walk(ast, (node) => {
    if (node.type === 'VariableDeclarator') {
      const id = node.id as Node;
      if (id.type === 'Identifier' && arrayHasT(node.init)) names.add(String(id.name));
    } else if (node.type === 'CallExpression' && calleeName(node) === 'push') {
      const callee = node.callee as Node;
      const obj = callee.type === 'MemberExpression' ? (callee.object as Node) : undefined;
      if (obj?.type === 'Identifier' && (node.arguments as unknown[]).some(tResult)) {
        names.add(String(obj.name));
      }
    }
    return undefined;
  });
  return names;
}

const ALLOWED_JOIN_SEPARATORS = new Set([' · ', '\n']);

function joinSeparator(call: Node): string | undefined {
  const sep = unwrap((call.arguments as unknown[])[0]);
  if (!isNode(sep)) return undefined;
  if (sep.type === 'StringLiteral') return String(sep.value);
  if (sep.type === 'TemplateLiteral' && (sep.expressions as unknown[]).length === 0) {
    return pieces(sep).join('');
  }
  return undefined;
}

const HAS_LETTER = /[A-Za-z]/;

/** User-facing wording: a letter, plus a space or a leading capital. Enum tokens and ids pass. */
function looksLikeText(n: Node): boolean {
  return pieces(n).some(
    (p, i) => HAS_LETTER.test(p) && (p.includes(' ') || (i === 0 && /^[A-Z]/.test(p))),
  );
}

function startsUpper(n: Node): boolean {
  return /^[A-Z]/.test(pieces(n)[0] ?? '');
}

function startsUpperWithSpace(n: Node): boolean {
  return startsUpper(n) && pieces(n).some((p) => p.includes(' '));
}

function propName(p: Node): string | undefined {
  if (p.computed === true) return undefined;
  const key = p.key as Node;
  if (key.type === 'Identifier') return str(key.name);
  if (key.type === 'StringLiteral') return str(key.value);
  return undefined;
}

function lineOf(n: Node, offset: number): number {
  return (n.loc?.start.line ?? 1) + offset;
}

class Collector {
  readonly hits: Hit[] = [];
  /** Literals a U67 check examined; U208 leaves them to U67. */
  readonly textOwned = new Set<Node>();
  constructor(
    readonly file: string,
    readonly ctx: ScanContext,
  ) {}
  add(rule: SourceRule, line: number, message: string): void {
    if (!this.hits.some((h) => h.line === line && h.rule === rule && h.message === message)) {
      this.hits.push({ file: this.file, line, rule, message });
    }
  }
}

// Placeholders every message gets from the app root (the active agent's names): a call site omits them.
const AMBIENT_PLACEHOLDERS = new Set(['agent', 'product', 'headless', 'command']);

function checkCatalogCall(c: Collector, call: Node, offset: number): void {
  const args = call.arguments as Node[];
  const first = args[0];
  if (first?.type !== 'StringLiteral') return;
  const key = String(first.value);
  const shape = c.ctx.messages.get(key);
  if (!shape) return;
  const line = lineOf(call, offset);
  const second = args[1];
  let provided: Set<string>;
  if (second === undefined) {
    provided = new Set();
  } else if (second.type === 'ObjectExpression') {
    provided = new Set();
    for (const p of second.properties as Node[]) {
      const name = p.type === 'ObjectProperty' ? propName(p) : undefined;
      if (name === undefined) return;
      provided.add(name);
    }
  } else {
    return;
  }
  if (shape.plural && !provided.has('count')) {
    c.add('U71', line, `t('${key}') is a plural message and needs a count param`);
    return;
  }
  const missing = [...shape.placeholders].filter(
    (p) => !provided.has(p) && !AMBIENT_PLACEHOLDERS.has(p),
  );
  const extra = [...provided].filter((p) => !shape.placeholders.has(p));
  if (missing.length > 0 || extra.length > 0) {
    const parts = [
      missing.length > 0 ? `missing {${missing.join('}, {')}}` : '',
      extra.length > 0 ? `unused ${extra.join(', ')}` : '',
    ].filter(Boolean);
    c.add(
      'U71',
      line,
      `t('${key}') params differ from the message placeholders: ${parts.join('; ')}`,
    );
  }
}

function checkJoin(c: Collector, call: Node, line: number, tArrays: ReadonlySet<string>): void {
  const callee = call.callee as Node;
  if (callee.type !== 'MemberExpression') return;
  const obj = unwrap(callee.object);
  const holdsT =
    arrayHasT(obj) || (isNode(obj) && obj.type === 'Identifier' && tArrays.has(String(obj.name)));
  if (!holdsT) return;
  const sep = joinSeparator(call);
  if (sep !== undefined && ALLOWED_JOIN_SEPARATORS.has(sep)) return;
  c.add(
    'U69',
    line,
    `t() results joined with ${sep === undefined ? 'a computed separator' : JSON.stringify(sep)}; sentences go through joinSentences(), facts through ' · ', lines through '\\n'`,
  );
}

function checkCompositionAndImports(
  c: Collector,
  node: Node,
  offset: number,
  tArrays: ReadonlySet<string>,
): void {
  const line = lineOf(node, offset);
  switch (node.type) {
    case 'CallExpression': {
      const name = calleeName(node);
      if (name === 'useI18n' || name === '$t' || name === '$tc') {
        c.add(
          'U68',
          line,
          `${name}( is reserved for packages/theme/src/i18n; use t() from @theme/i18n`,
        );
      }
      if (name === 'plural')
        c.add('U69', line, 'plural( builds text; use a plural message with t(key, { count })');
      if (name === 'copyAnnounced') {
        const second = (node.arguments as Node[])[1];
        if (second && terminalLiterals(second).length > 0) {
          c.add(
            'U69',
            line,
            'copyAnnounced(text, "<word>") composes a sentence; pass a whole message',
          );
        }
      }
      if (isIdentCall(node, 't')) checkCatalogCall(c, node, offset);
      if (name === 'join') checkJoin(c, node, line, tArrays);
      break;
    }
    case 'MemberExpression': {
      const obj = node.object as Node;
      const prop = node.property as Node;
      if (
        obj.type === 'Identifier' &&
        obj.name === 'i18n' &&
        prop.name === 'global' &&
        node.computed !== true
      ) {
        c.add('U68', line, 'i18n.global is reserved for packages/theme/src/i18n');
      }
      break;
    }
    case 'ImportDeclaration':
      if ((node.source as Node).value === 'vue-i18n') {
        c.add('U68', line, "import from 'vue-i18n' is reserved for packages/theme/src/i18n");
      }
      break;
    case 'BinaryExpression':
      if (node.operator === '+' && (tResult(node.left) || tResult(node.right))) {
        c.add('U69', line, 't() result concatenated with +; one message with a placeholder');
      }
      break;
    case 'TemplateLiteral':
      if (isComposedTemplate(node)) {
        c.add(
          'U69',
          line,
          't() result composed in a template literal; use a placeholder or pair message (common.dotPair, common.withDetail) or joinSentences()',
        );
      }
      break;
    default:
  }
}

function checkTextProperty(
  c: Collector,
  node: Node,
  ancestors: readonly Node[],
  offset: number,
): void {
  const name = propName(node);
  const objectParent = ancestors[ancestors.length - 1];
  const grand = ancestors[ancestors.length - 2];
  const inTParams =
    grand !== undefined && isIdentCall(grand, 't') && objectParent?.type === 'ObjectExpression';
  if (name === undefined || !TEXT_PROPS.has(name) || inTParams) return;
  // A cva variant key (`title: '…'` under `variants`) names a variant, not text.
  if (ancestors.some((a) => isIdentCall(a, 'cva'))) return;
  for (const lit of terminalLiterals(node.value)) {
    c.textOwned.add(lit);
    if (looksLikeText(lit)) {
      c.add('U67', lineOf(lit, offset), `literal text in "${name}"; move to the catalog`);
    }
  }
}

function checkTextSink(c: Collector, node: Node, offset: number): void {
  const name = calleeName(node);
  if (name === undefined || !TEXT_SINKS.has(name)) return;
  for (const arg of node.arguments as Node[]) {
    for (const lit of terminalLiterals(arg)) {
      c.textOwned.add(lit);
      if (startsUpper(lit)) {
        c.add('U67', lineOf(lit, offset), `literal text passed to ${name}(; move to the catalog`);
      }
    }
  }
}

function checkNewError(c: Collector, node: Node, offset: number): void {
  const callee = node.callee as Node;
  if (callee.type !== 'Identifier' || callee.name !== 'Error') return;
  for (const lit of terminalLiterals((node.arguments as Node[])[0])) {
    c.textOwned.add(lit);
    if (startsUpper(lit)) {
      c.add(
        'U67',
        lineOf(lit, offset),
        'new Error( with user-facing text; developer errors use a lowercase "fn:" prefix',
      );
    }
  }
}

function checkStateAssignment(c: Collector, node: Node, offset: number): void {
  if (node.operator !== '=' || !assignsStateText(node.left as Node)) return;
  for (const lit of terminalLiterals(node.right)) {
    c.textOwned.add(lit);
    if (startsUpperWithSpace(lit)) {
      c.add('U67', lineOf(lit, offset), 'literal text assigned to state; move to the catalog');
    }
  }
}

const NO_ARRAYS: ReadonlySet<string> = new Set();

const PROSE_WORD = /^[‘'“"(]?[A-Za-z][a-z’']*[,.;:!?)’'”"]*$/;
const PROSE_BANNED = /[[\]{}<>=|`]/;
const FUNCTION_WORDS = new Set(
  'the a an is are was were be been not no has have had this that it its to of for with could can does did will would must there your you'.split(
    ' ',
  ),
);

/** Prose position: property value, return, arrow body, `??`/`||` right side, array element, branch. */
function inProseSlot(n: Node, parent: Node | null): boolean {
  if (parent === null) return false;
  switch (parent.type) {
    case 'ObjectProperty':
      return parent.value === n && !TEXT_PROPS.has(propName(parent) ?? '');
    case 'ReturnStatement':
      return parent.argument === n;
    case 'ArrowFunctionExpression':
      return parent.body === n;
    case 'LogicalExpression':
      return (parent.operator === '??' || parent.operator === '||') && parent.right === n;
    case 'ArrayExpression':
      return true;
    case 'ConditionalExpression':
      return parent.consequent === n || parent.alternate === n;
    default:
      return false;
  }
}

function looksLikeProse(n: Node): boolean {
  const text = pieces(n).join('x');
  if (!/^[A-Za-z‘'“"]/.test(text) || PROSE_BANNED.test(text)) return false;
  const words = text.split(/\s+/).filter((w) => PROSE_WORD.test(w));
  return words.length >= 3 && words.some((w) => FUNCTION_WORDS.has(w.toLowerCase()));
}

function checkProse(c: Collector, node: Node, parent: Node | null, offset: number): void {
  if (c.textOwned.has(node) || !inProseSlot(node, parent) || !looksLikeProse(node)) return;
  c.add(
    'U208',
    lineOf(node, offset),
    'English prose outside the catalog; move it to en.json and call t() at call time',
  );
}

function scanScript(c: Collector, ast: unknown, offset: number, moduleScope: boolean): void {
  const tArrays = collectTArrays(ast);
  walk(ast, (node, _parent, ancestors) => {
    checkCompositionAndImports(c, node, offset, tArrays);
    if (
      moduleScope &&
      isIdentCall(node, 't') &&
      !ancestors.some((a) => FUNCTION_SCOPES.has(a.type))
    ) {
      c.add(
        'U70',
        lineOf(node, offset),
        't() at module scope; keep the key and call t() at render or call time',
      );
    }
    if (node.type === 'ObjectProperty') checkTextProperty(c, node, ancestors, offset);
    else if (node.type === 'CallExpression') checkTextSink(c, node, offset);
    else if (node.type === 'NewExpression') checkNewError(c, node, offset);
    else if (node.type === 'AssignmentExpression') checkStateAssignment(c, node, offset);
    if (node.type === 'StringLiteral' || node.type === 'TemplateLiteral') {
      checkProse(c, node, _parent, offset);
    }
    return undefined;
  });
}

function assignsStateText(left: Node): boolean {
  if (left.type !== 'MemberExpression') return false;
  const prop = left.property as Node;
  if (left.computed !== true && prop.name === 'value') return true;
  let root: Node = left;
  while (root.type === 'MemberExpression') root = root.object as Node;
  return root.type === 'Identifier' && root.name === 'state';
}

/** `text` mode also applies U66 (interpolation or text-bound attribute). */
function scanTemplateExpression(
  c: Collector,
  ast: unknown,
  offset: number,
  textMode: boolean,
): void {
  walk(ast, (node, parent) => {
    checkCompositionAndImports(c, node, offset, NO_ARRAYS);
    if (!textMode || (node.type !== 'StringLiteral' && node.type !== 'TemplateLiteral'))
      return undefined;
    if (!looksLikeText(node)) return undefined;
    if (parent && isIdentCall(parent, 't') && (parent.arguments as unknown[])[0] === node)
      return undefined;
    if (parent?.type === 'BinaryExpression' && EQUALITY_OPS.has(String(parent.operator)))
      return undefined;
    if (parent?.type === 'MemberExpression' && parent.computed === true) return undefined;
    if (parent?.type === 'TSEnumMember') return undefined;
    c.add(
      'U66',
      lineOf(node, offset),
      'literal text in a template expression; move to the catalog',
    );
    return undefined;
  });
}

function parseExpr(code: string): Node | null {
  try {
    return parseExpression(code, { plugins: ['typescript'] }) as unknown as Node;
  } catch {
    try {
      return parseBabel(code, { sourceType: 'module', plugins: ['typescript'] })
        .program as unknown as Node;
    } catch {
      return null;
    }
  }
}

function isTextAttr(rawName: string): boolean {
  if (rawName.startsWith('data-')) return false;
  const name = rawName.replace(/[A-Z]/g, (m) => `-${m.toLowerCase()}`);
  return TEXT_ATTRS.has(name) || TEXT_ATTR_SUFFIX.test(name);
}

function scanExp(c: Collector, exp: ExpressionNode | undefined, textMode: boolean): void {
  if (exp?.type !== NodeTypes.SIMPLE_EXPRESSION || exp.content.trim() === '') return;
  const ast = parseExpr(exp.content);
  if (ast) scanTemplateExpression(c, ast, exp.loc.start.line - 1, textMode);
}

function scanProps(c: Collector, el: ElementNode): void {
  for (const p of el.props) {
    const line = p.loc.start.line;
    if (p.type === NodeTypes.ATTRIBUTE) {
      if (isTextAttr(p.name) && HAS_LETTER.test(p.value?.content ?? '')) {
        c.add('U65', line, `static text in "${p.name}"; bind a catalog message`);
      }
    } else if (p.exp) {
      if (p.name === 'for' || p.name === 'slot') continue;
      const arg = p.arg?.type === NodeTypes.SIMPLE_EXPRESSION && p.arg.isStatic ? p.arg : undefined;
      const textual = p.name === 'bind' && arg !== undefined && isTextAttr(arg.content);
      scanExp(c, p.exp, textual);
    }
  }
}

function hasSlotDirective(el: ElementNode): boolean {
  return el.props.some((p) => p.type === NodeTypes.DIRECTIVE && p.name === 'slot');
}

function scanTemplateNode(
  c: Collector,
  node: RootNode | TemplateChildNode,
  parentTag: string | undefined,
  inCatalogSlot: boolean,
): void {
  const line = node.loc.start.line;
  if (node.type === NodeTypes.TEXT) {
    const text = node.content;
    const exempt = inCatalogSlot && (parentTag === 'code' || parentTag === 'kbd');
    if (HAS_LETTER.test(text) && !exempt) {
      c.add('U64', line, `literal text "${text.trim().slice(0, 40)}"; move to the catalog`);
    }
    return;
  }
  if (node.type === NodeTypes.INTERPOLATION) {
    scanExp(c, node.content, true);
    return;
  }
  if (node.type === NodeTypes.ELEMENT) {
    const tag = node.tag;
    if (I18N_TAGS.has(tag))
      c.add('U68', line, `<${tag}> is reserved for packages/theme/src/i18n; use CatalogText`);
    scanProps(c, node);
    const slot =
      inCatalogSlot ||
      (tag === 'template' &&
        hasSlotDirective(node) &&
        parentTag !== undefined &&
        CATALOG_TEXT_TAGS.has(parentTag));
    for (const child of node.children) scanTemplateNode(c, child, tag, slot);
    return;
  }
  if (node.type === NodeTypes.ROOT) {
    for (const child of node.children) scanTemplateNode(c, child, undefined, inCatalogSlot);
  }
}

function parseTs(code: string): Node {
  return parseBabel(code, {
    sourceType: 'module',
    plugins: ['typescript'],
    allowReturnOutsideFunction: true,
  }).program as unknown as Node;
}

export function scanSource(file: string, source: string, ctx: ScanContext): Hit[] {
  const c = new Collector(file, ctx);
  try {
    if (file.endsWith('.vue')) {
      const { descriptor } = parseSFC(source, { filename: file });
      if (descriptor.template?.ast) {
        // The template tag's own children carry file-relative lines.
        for (const child of descriptor.template.ast.children) {
          scanTemplateNode(c, child, undefined, false);
        }
      }
      if (descriptor.script) {
        scanScript(
          c,
          parseTs(descriptor.script.content),
          descriptor.script.loc.start.line - 1,
          true,
        );
      }
      if (descriptor.scriptSetup) {
        scanScript(
          c,
          parseTs(descriptor.scriptSetup.content),
          descriptor.scriptSetup.loc.start.line - 1,
          false,
        );
      }
    } else {
      scanScript(c, parseTs(source), 0, true);
    }
  } catch (e) {
    throw new Error(`${file}: parse failed: ${(e as Error).message}`);
  }
  return c.hits.sort((a, b) => a.line - b.line || a.rule.localeCompare(b.rule));
}
