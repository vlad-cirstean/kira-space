import type { StatusColor } from './wire';

// Class maps over --color-status-* (packages/theme tokens.css). Tailwind emits only literals, so each
// class is spelled in full.

/** The palette in display order. */
export const STATUS_COLORS: readonly StatusColor[] = [
  'grey',
  'brown',
  'red',
  'orange',
  'amber',
  'yellow',
  'lime',
  'green',
  'teal',
  'cyan',
  'sky',
  'blue',
  'indigo',
  'violet',
  'purple',
  'pink',
];

/** Chip: tinted background, coloured text. */
export const STATUS_CHIP_CLASS: Record<StatusColor, string> = {
  grey: 'bg-status-grey/15 text-status-grey',
  brown: 'bg-status-brown/15 text-status-brown',
  red: 'bg-status-red/15 text-status-red',
  orange: 'bg-status-orange/15 text-status-orange',
  amber: 'bg-status-amber/15 text-status-amber',
  yellow: 'bg-status-yellow/15 text-status-yellow',
  lime: 'bg-status-lime/15 text-status-lime',
  green: 'bg-status-green/15 text-status-green',
  teal: 'bg-status-teal/15 text-status-teal',
  cyan: 'bg-status-cyan/15 text-status-cyan',
  sky: 'bg-status-sky/15 text-status-sky',
  blue: 'bg-status-blue/15 text-status-blue',
  indigo: 'bg-status-indigo/15 text-status-indigo',
  violet: 'bg-status-violet/15 text-status-violet',
  purple: 'bg-status-purple/15 text-status-purple',
  pink: 'bg-status-pink/15 text-status-pink',
};

/** Solid dot. */
export const STATUS_DOT_CLASS: Record<StatusColor, string> = {
  grey: 'bg-status-grey',
  brown: 'bg-status-brown',
  red: 'bg-status-red',
  orange: 'bg-status-orange',
  amber: 'bg-status-amber',
  yellow: 'bg-status-yellow',
  lime: 'bg-status-lime',
  green: 'bg-status-green',
  teal: 'bg-status-teal',
  cyan: 'bg-status-cyan',
  sky: 'bg-status-sky',
  blue: 'bg-status-blue',
  indigo: 'bg-status-indigo',
  violet: 'bg-status-violet',
  purple: 'bg-status-purple',
  pink: 'bg-status-pink',
};

/** Colour picker swatch: the dot, kept in its colour when checked. */
export const STATUS_SWATCH_CLASS: Record<StatusColor, string> = {
  grey: 'bg-status-grey data-[state=checked]:bg-status-grey',
  brown: 'bg-status-brown data-[state=checked]:bg-status-brown',
  red: 'bg-status-red data-[state=checked]:bg-status-red',
  orange: 'bg-status-orange data-[state=checked]:bg-status-orange',
  amber: 'bg-status-amber data-[state=checked]:bg-status-amber',
  yellow: 'bg-status-yellow data-[state=checked]:bg-status-yellow',
  lime: 'bg-status-lime data-[state=checked]:bg-status-lime',
  green: 'bg-status-green data-[state=checked]:bg-status-green',
  teal: 'bg-status-teal data-[state=checked]:bg-status-teal',
  cyan: 'bg-status-cyan data-[state=checked]:bg-status-cyan',
  sky: 'bg-status-sky data-[state=checked]:bg-status-sky',
  blue: 'bg-status-blue data-[state=checked]:bg-status-blue',
  indigo: 'bg-status-indigo data-[state=checked]:bg-status-indigo',
  violet: 'bg-status-violet data-[state=checked]:bg-status-violet',
  purple: 'bg-status-purple data-[state=checked]:bg-status-purple',
  pink: 'bg-status-pink data-[state=checked]:bg-status-pink',
};

/** Coloured text. */
export const STATUS_TEXT_CLASS: Record<StatusColor, string> = {
  grey: 'text-status-grey',
  brown: 'text-status-brown',
  red: 'text-status-red',
  orange: 'text-status-orange',
  amber: 'text-status-amber',
  yellow: 'text-status-yellow',
  lime: 'text-status-lime',
  green: 'text-status-green',
  teal: 'text-status-teal',
  cyan: 'text-status-cyan',
  sky: 'text-status-sky',
  blue: 'text-status-blue',
  indigo: 'text-status-indigo',
  violet: 'text-status-violet',
  purple: 'text-status-purple',
  pink: 'text-status-pink',
};
