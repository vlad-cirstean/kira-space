import type { StatusTone } from '@theme/lib/statusTone';
import type { Tone } from './board/actions';

// Class maps over the tokens in tones.css (built on theme tokens). Tailwind emits only literals, so each is spelled in full.
export const TONE_TEXT_CLASS: Record<Tone, string> = {
  amber: 'text-tone-amber',
  red: 'text-tone-red',
  green: 'text-tone-green',
  blue: 'text-tone-blue',
  purple: 'text-tone-purple',
  grey: 'text-tone-grey',
};

/** Tag pill (tint background, tone text). */
export const TONE_TAG_CLASS: Record<Tone, string> = {
  amber: 'bg-tone-amber-tint text-tone-amber',
  red: 'bg-tone-red-tint text-tone-red',
  green: 'bg-tone-green-tint text-tone-green',
  blue: 'bg-tone-blue-tint text-tone-blue',
  purple: 'bg-tone-purple-tint text-tone-purple',
  grey: 'bg-tone-grey-tint text-tone-grey',
};

/** Solid indicator fill (never a button; fixed ink: a themed ink loses contrast under another theme). */
export const SOLID_CLASS: Record<Tone, string> = {
  amber: 'bg-tone-amber-solid text-tone-ink border-transparent',
  red: 'bg-tone-red-solid text-tone-ink border-transparent',
  green: 'bg-tone-green-solid text-tone-ink border-transparent',
  blue: 'bg-tone-blue-solid text-tone-ink border-transparent',
  purple: 'bg-tone-purple-solid text-tone-ink border-transparent',
  grey: 'bg-tone-grey-solid text-tone-ink border-transparent',
};

// Off-board render of a board tone (D1). Purple is merged on the board.
export const THEME_TONE: Record<Tone, StatusTone> = {
  amber: 'warn',
  red: 'error',
  green: 'ok',
  blue: 'info',
  purple: 'ok',
  grey: 'neutral',
};
