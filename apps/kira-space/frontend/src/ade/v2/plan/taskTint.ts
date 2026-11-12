import type { StyleValue } from 'vue';

/**
 * Presentation of a task's colour: a faint fill and a tinted border on its whole box. Inline style,
 * since Tailwind cannot generate one class per user-picked colour. `selected` keeps the focus border.
 */
export function taskTint(color: string | null | undefined, selected: boolean): StyleValue {
  if (!color) return {};
  return {
    backgroundColor: `color-mix(in oklab, ${color} 10%, var(--color-elevated))`,
    borderColor: selected ? undefined : `color-mix(in oklab, ${color} 45%, var(--color-border))`,
  };
}
