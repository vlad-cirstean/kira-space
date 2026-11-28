import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { PALETTE, TASK_BAND, TASK_BAND_HOVER } from '../../frontend/src/ade/v2/palette';

// P764 guard: the 10-slot task palette keeps text readable on every header band and keeps the bands
// apart. Own OKLab and WCAG helpers, no dependency.

const tokens = readFileSync(
  new URL('../../../../packages/theme/src/tokens.css', import.meta.url),
  'utf8',
);
const token = (name: string): string => {
  const m = tokens.match(new RegExp(`--kira-${name}:\\s*([^;]+);`));
  if (!m?.[1]) throw new Error(`token ${name} missing`);
  return m[1].trim();
};
const num = (name: string): number => Number.parseFloat(token(name));

// Hue of each slot, 36 deg apart from red 20, in slot order (palette.ts comment).
const HUES = [20, 200, 92, 308, 272, 56, 344, 164, 128, 236];

type Rgb = [number, number, number];
const rgbOf = (hex: string): Rgb =>
  [1, 3, 5].map((i) => Number.parseInt(hex.slice(i, i + 2), 16)) as Rgb;
const lin = (c: number): number => {
  const v = c / 255;
  return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
};
const luminance = (hex: string): number => {
  const [r, g, b] = rgbOf(hex);
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
};
const contrast = (a: string, b: string): number => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
};
type Lab = [number, number, number];
const oklab = (hex: string): Lab => {
  const [r, g, b] = rgbOf(hex).map(lin) as Rgb;
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
};
const dist = (a: string, b: string): number => {
  const [p, q] = [oklab(a), oklab(b)];
  return Math.hypot(p[0] - q[0], p[1] - q[1], p[2] - q[2]);
};
const chromaOf = (hex: string): number => Math.hypot(oklab(hex)[1], oklab(hex)[2]);

const gamma = (v: number): number => (v <= 0.0031308 ? 12.92 * v : 1.055 * v ** (1 / 2.4) - 0.055);
// OKLCH to sRGB channels (unclamped, 0-1).
const srgbOf = (L: number, C: number, hue: number): Rgb => {
  const a = C * Math.cos((hue * Math.PI) / 180);
  const b = C * Math.sin((hue * Math.PI) / 180);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    gamma(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
    gamma(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
    gamma(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s),
  ];
};
const inGamut = (c: Rgb): boolean => c.every((v) => v >= -0.0005 && v <= 1.0005);
/** Band recipe: fixed lightness, chroma clamped down until the colour fits sRGB. */
const recipe = (L: number, C: number, hue: number): Rgb => {
  let lo = 0;
  let hi = C;
  if (inGamut(srgbOf(L, hi, hue))) lo = hi;
  else
    for (let i = 0; i < 30; i++) {
      const mid = (lo + hi) / 2;
      if (inGamut(srgbOf(L, mid, hue))) lo = mid;
      else hi = mid;
    }
  return srgbOf(L, lo, hue).map((v) => Math.round(Math.min(1, Math.max(0, v)) * 255)) as Rgb;
};

const bg = token('bg');
const chrome = token('bg-chrome');
const TEXT = ['fg', 'fg-muted', 'fg-subtle'].map(token);
const pairs = (list: readonly string[]): number[] =>
  list.flatMap((a, i) => list.slice(i + 1).map((b) => dist(a, b)));

describe('task palette', () => {
  test('10 inks, 10 rest bands, 10 hover bands', () => {
    expect(PALETTE.length).toBe(10);
    expect(TASK_BAND.length).toBe(10);
    expect(TASK_BAND_HOVER.length).toBe(10);
    expect(bg).toMatch(/^#/);
  });

  test('band hex matches the OKLCH recipe in tokens.css', () => {
    const cases = [
      [TASK_BAND, num('task-band-l'), num('task-band-c')],
      [TASK_BAND_HOVER, num('task-band-l-hover'), num('task-band-c-hover')],
    ] as const;
    for (const [bands, L, C] of cases) {
      bands.forEach((hex, i) => {
        const want = recipe(L, C, HUES[i] as number);
        const got = rgbOf(hex);
        for (let k = 0; k < 3; k++)
          expect(Math.abs((got[k] as number) - (want[k] as number))).toBeLessThanOrEqual(1);
      });
    }
  });

  test('text stays readable on every band', () => {
    for (const band of [...TASK_BAND, ...TASK_BAND_HOVER])
      for (const fg of TEXT) expect(contrast(fg, band)).toBeGreaterThanOrEqual(4.5);
  });

  test('rest bands stay apart', () => {
    expect(Math.min(...pairs(TASK_BAND))).toBeGreaterThanOrEqual(0.018);
    const adjacent = TASK_BAND.slice(1).map((hex, i) => dist(hex, TASK_BAND[i] as string));
    expect(Math.min(...adjacent)).toBeGreaterThanOrEqual(0.02);
  });

  test('inks read on chrome and share one weight', () => {
    for (const ink of PALETTE) expect(contrast(ink, chrome)).toBeGreaterThanOrEqual(4.5);
    const cs = PALETTE.map(chromaOf);
    const ls = PALETTE.map((h) => oklab(h)[0]);
    expect(Math.max(...cs) - Math.min(...cs)).toBeLessThanOrEqual(0.1);
    expect(Math.max(...ls) - Math.min(...ls)).toBeLessThanOrEqual(0.1);
  });
});
