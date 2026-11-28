// Work colours: `Task.color` is a slot 0-9, assigned server-side. Slots run farthest-first on the rest
// band, so the first live tasks and consecutive slots sit maximally apart.
// Inks: OKLCH L 0.68 (amber 0.76) C 0.13 gamut-clamped, hue 36 deg apart from red 20.
// Bands: OKLCH L 0.235 C 0.035 rest, L 0.250 C 0.045 hover/selected, gamut-clamped per hue. Hex is
// precomputed (WebKit and Chromium gamut-map oklch differently); the recipe tokens in tokens.css and
// tests/unit/ade-v2-palette-contrast.spec.ts guard it.
// Slot:  0 red, 1 teal, 2 amber, 3 violet, 4 blue, 5 orange, 6 magenta, 7 green, 8 lime, 9 sky.
export const PALETTE = [
  '#dd7577',
  '#09adb4',
  '#cfae42',
  '#af81d6',
  '#7d92e8',
  '#d48140',
  '#cf76ac',
  '#31b182',
  '#81a749',
  '#31a3dd',
] as const;

export const TASK_BAND = [
  '#2d1717',
  '#052325',
  '#241e09',
  '#23192b',
  '#181d2f',
  '#2b1a0d',
  '#2a1722',
  '#0c231a',
  '#19210f',
  '#0c212c',
] as const;

export const TASK_BAND_HOVER = [
  '#341819',
  '#012729',
  '#292105',
  '#281b32',
  '#1a2037',
  '#321b0a',
  '#311827',
  '#08281c',
  '#1b260d',
  '#082534',
] as const;

const slotOf = (slot: number): number =>
  ((slot % PALETTE.length) + PALETTE.length) % PALETTE.length;

export function taskColor(slot: number): string {
  return PALETTE[slotOf(slot)] as string;
}

/** Header band ground of a task: `rest`, and `hover` for a hovered or selected head. */
export function taskBand(slot: number): { rest: string; hover: string } {
  const i = slotOf(slot);
  return { rest: TASK_BAND[i] as string, hover: TASK_BAND_HOVER[i] as string };
}
