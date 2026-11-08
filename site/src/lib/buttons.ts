/** Tailwind classes shared by the site's call-to-action buttons. */
const base =
  'group flex items-center gap-2 rounded-xl font-semibold focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent motion-safe:transition-[transform,box-shadow] motion-safe:hover:-translate-y-0.5';
const sizes = {
  md: 'min-h-12 px-6 text-base',
  sm: 'min-h-11 px-3 text-sm sm:px-4 sm:text-base',
} as const;
const variants = {
  primary:
    'bg-linear-to-b from-brand-sky to-brand-deep text-white shadow-cta hover:shadow-cta-hover',
  secondary: 'bg-white/5 text-fg border border-white/12 hover:bg-white/10 hover:border-white/25',
} as const;

export function button(variant: keyof typeof variants, size: keyof typeof sizes = 'md'): string {
  return `${base} ${sizes[size]} ${variants[variant]}`;
}
