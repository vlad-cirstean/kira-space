import type { ListGridConstructorOptions } from 'cheetah-grid';

export type ThemeDefine = Exclude<
  NonNullable<ListGridConstructorOptions<unknown>['theme']>,
  string
>;

type ColorFn = (args: { col: number; row: number }) => string;

export interface Palette {
  bg: string;
  bgChrome: string;
  fg: string;
  fgMuted: string;
  fgSubtle: string;
  border: string;
  select: string;
  hover: string;
  focus: string;
  warn: string;
  ok: string;
  error: string;
  searchMatch: string;
  searchCurrent: string;
  number: string;
  keyword: string;
  control: string;
  font: string;
  fontItalic: string;
  fontBold: string;
  fontSize: number;
  rowHeight: number;
}

let probeEl: HTMLSpanElement | null = null;
let probeCtx: CanvasRenderingContext2D | null = null;

/** Canvas cannot parse `color-mix()` or `var()`. The probe element resolves a token through the
 *  cascade, then one painted pixel turns the result into a plain `rgba()` string. */
function resolveColor(value: string): string {
  if (!probeEl) {
    probeEl = document.createElement('span');
    probeEl.style.display = 'none';
    document.body.appendChild(probeEl);
  }
  if (!probeCtx) {
    const canvas = document.createElement('canvas');
    canvas.width = 1;
    canvas.height = 1;
    probeCtx = canvas.getContext('2d', { willReadFrequently: true });
  }
  if (!probeCtx) throw new Error('2D canvas context is unavailable');
  probeEl.style.color = '';
  probeEl.style.color = value;
  const computed = getComputedStyle(probeEl).color;
  probeCtx.clearRect(0, 0, 1, 1);
  probeCtx.fillStyle = computed;
  probeCtx.fillRect(0, 0, 1, 1);
  const px = probeCtx.getImageData(0, 0, 1, 1).data;
  return `rgba(${px[0]}, ${px[1]}, ${px[2]}, ${((px[3] as number) / 255).toFixed(3)})`;
}

export function readPalette(rowHeightOverride?: number): Palette {
  const root = getComputedStyle(document.documentElement);
  const token = (name: string): string => root.getPropertyValue(name).trim();
  const color = (name: string): string => resolveColor(token(name));
  const size = Number.parseFloat(token('--kira-font-size')) || 12;
  const family = token('--kira-font-data') || 'monospace';
  const rowHeight = rowHeightOverride ?? (Number.parseFloat(token('--kira-row-height')) || 28);
  return {
    bg: color('--kira-bg'),
    bgChrome: color('--kira-bg-chrome'),
    fg: color('--kira-fg'),
    fgMuted: color('--kira-fg-muted'),
    fgSubtle: color('--kira-fg-subtle'),
    border: color('--kira-border'),
    select: color('--kira-select'),
    hover: color('--kira-bg-input'),
    focus: color('--kira-focus'),
    warn: color('--kira-warn'),
    ok: color('--kira-ok'),
    error: color('--kira-error'),
    searchMatch: color('--kira-search-match'),
    searchCurrent: color('--kira-search-match-current'),
    number: color('--kira-syntax-number'),
    keyword: color('--kira-syntax-keyword'),
    control: color('--kira-syntax-control'),
    font: `${size}px ${family}`,
    fontItalic: `italic ${size}px ${family}`,
    fontBold: `600 ${size}px ${family}`,
    fontSize: size,
    rowHeight,
  };
}

/** `cellBg` paints every cell background (selection, search, hover, zebra all fold into it), so
 *  default, selected and active cells share one function and no layer needs a second pass. */
export function buildTheme(p: Palette, cellBg: ColorFn): ThemeDefine {
  return {
    font: p.font,
    underlayBackgroundColor: p.bg,
    color: p.fg,
    frozenRowsColor: p.fgMuted,
    defaultBgColor: cellBg,
    frozenRowsBgColor: p.bgChrome,
    selectionBgColor: cellBg,
    highlightBgColor: cellBg,
    borderColor: p.border,
    frozenRowsBorderColor: p.border,
    highlightBorderColor: p.border,
    checkbox: {},
    radioButton: {},
    button: {},
    tree: {},
    header: { sortArrowColor: p.fgMuted },
    messages: {},
    indicators: {},
  };
}
