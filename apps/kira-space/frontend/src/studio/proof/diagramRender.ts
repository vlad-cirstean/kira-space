import { errorText } from '@workbench/bridge/codedError';
import type { ChartTheme } from '../charts/theme';

const MAX_SVG_BYTES = 2 << 20;
const MAX_ERROR_CHARS = 500;
// Mermaid's default locked keys plus the ones this renderer sets, so a diagram cannot override them.
const SECURE_KEYS = [
  'secure',
  'securityLevel',
  'startOnLoad',
  'maxTextSize',
  'suppressErrorRendering',
  'maxEdges',
  'theme',
  'themeVariables',
  'htmlLabels',
  'layout',
  'fontFamily',
];

export class DiagramTooLargeError extends Error {}

let counter = 0;
// mermaid.initialize is global, so renders run one at a time.
let queue: Promise<unknown> = Promise.resolve();
let purifier: Promise<import('dompurify').DOMPurify> | null = null;

function isDark(hex: string): boolean {
  const m = /^#([0-9a-f]{6})$/i.exec(hex);
  if (!m?.[1]) return false;
  const n = Number.parseInt(m[1], 16);
  const luma = 0.2126 * (n >> 16) + 0.7152 * ((n >> 8) & 0xff) + 0.0722 * (n & 0xff);
  return luma < 128;
}

// A private instance: mermaid uses the default one internally, and a hook there would change its
// label handling.
function privatePurifier(): Promise<import('dompurify').DOMPurify> {
  purifier ??= import('dompurify').then(({ default: create }) => {
    const p = create(window);
    p.addHook('uponSanitizeAttribute', (_node, data) => {
      if (
        (data.attrName === 'href' || data.attrName === 'xlink:href') &&
        !data.attrValue.startsWith('#')
      ) {
        data.keepAttr = false;
      }
    });
    return p;
  });
  return purifier;
}

async function run(source: string, theme: ChartTheme, id: string): Promise<string> {
  const [{ default: mermaid }, purify] = await Promise.all([import('mermaid'), privatePurifier()]);
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    htmlLabels: false,
    maxTextSize: 20_000,
    maxEdges: 500,
    layout: 'dagre',
    theme: 'base',
    fontFamily: theme.fontFamily,
    suppressErrorRendering: true,
    secure: SECURE_KEYS,
    themeVariables: {
      darkMode: isDark(theme.tooltipBackground),
      primaryColor: theme.tooltipBackground,
      secondaryColor: theme.tooltipBackground,
      tertiaryColor: theme.tooltipBackground,
      primaryTextColor: theme.text,
      textColor: theme.text,
      primaryBorderColor: theme.axisLine,
      lineColor: theme.mutedText,
      fontFamily: theme.fontFamily,
      fontSize: '13px',
    },
  });
  await mermaid.parse(source);
  const { svg } = await mermaid.render(id, source);
  const clean = purify.sanitize(svg, {
    USE_PROFILES: { svg: true, svgFilters: true },
    ADD_TAGS: ['style'],
    FORBID_TAGS: ['script', 'foreignObject', 'image', 'a', 'iframe'],
  });
  if (clean.length > MAX_SVG_BYTES) throw new DiagramTooLargeError('too large');
  return clean;
}

/** Mermaid text to sanitized SVG. Throws an Error carrying the parser message on failure. */
export function renderDiagram(source: string, theme: ChartTheme): Promise<string> {
  const id = `proof-diagram-${++counter}`;
  const next = queue.then(() => run(source, theme, id));
  // The caller sees the failure; the chain only needs to stay usable.
  queue = next.then(
    () => undefined,
    () => undefined,
  );
  return next.catch((err: unknown) => {
    if (err instanceof DiagramTooLargeError) throw err;
    throw new Error(errorText(err).slice(0, MAX_ERROR_CHARS));
  });
}
