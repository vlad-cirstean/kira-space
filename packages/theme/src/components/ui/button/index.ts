import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Button } from '@theme/components/ui/button/Button.vue';

export const buttonVariants = cva(
  'aria-invalid:border-error rounded-kira border border-transparent bg-clip-padding text-kira-md font-medium active:not-aria-[haspopup]:translate-y-px [&_svg:not([class*=size-])]:size-4 group/button inline-flex shrink-0 items-center justify-center whitespace-nowrap transition-all select-none disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        ghost: 'hover:bg-field hover:text-fg aria-expanded:bg-field aria-expanded:text-fg',
        link: 'cursor-pointer text-primary underline-offset-4 hover:underline',
        // AppButton's `kind`/`variant` vocabulary, collapsed onto this cva's own
        // `variant` axis instead of a second wrapper prop layer.
        // Geometry stays on the `kira`/`kira-lg`/`kira-icon` sizes below (design-system tokens,
        // runtime-adjustable via @theme's --spacing-control* -- see base.css).
        toolbar:
          'rounded-kira-sm text-muted-foreground hover:bg-field hover:text-fg aria-pressed:bg-field aria-pressed:text-fg data-[tone=error]:text-error data-[tone=error]:hover:text-error data-[tone=ok]:text-ok data-[tone=ok]:hover:text-ok',
        'toolbar-primary': 'rounded-kira-sm bg-primary text-primary-foreground hover:bg-primary/80',
        dialog:
          'justify-center rounded-kira-sm border border-border bg-field text-fg hover:bg-field',
        'dialog-primary':
          'justify-center rounded-kira-sm border border-primary bg-primary text-primary-foreground hover:bg-primary/80',
        'dialog-danger':
          'justify-center rounded-kira-sm border border-error bg-error/10 text-error hover:bg-error/20',
        danger: 'rounded-kira-sm text-error hover:bg-field',
        // P110 B13: TitleBar.vue's own action row -- `aria-pressed` replaces `.is-on`'s own class
        // toggle, fixing the state's previously unexposed accessibility gap.
        title:
          'rounded-kira-sm border border-transparent bg-transparent text-muted-foreground hover:bg-field wails-no-drag aria-pressed:bg-elevated aria-pressed:text-fg aria-pressed:hover:bg-field',
      },
      size: {
        'icon-sm':
          'size-7 rounded-[min(var(--kira-radius-sm),12px)] in-data-[slot=button-group]:rounded-kira',
        // P104 §3: AppButton/IconButton's own runtime-adjustable control heights (base.css's
        // --spacing-control*, an @theme-indirected --kira-control-h*) track the Appearance density.
        kira: 'h-control gap-1.5 px-3 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        'kira-lg':
          'h-control-lg gap-1.5 px-3 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        'kira-icon': 'size-control',
        // Status-bar items, mobile touch targets and bare inline buttons: one size per role.
        status: 'h-control-sm gap-1 px-1.5 font-ui text-kira-sm font-normal tabular-nums',
        touch: 'h-touch px-4 text-kira-touch',
        'touch-icon': 'size-touch',
        inline: 'h-auto p-0',
        // P110 B13
        title: 'size-5.5',
        'title-labelled': 'h-5.5 px-1 gap-1',
      },
    },
    defaultVariants: {
      variant: 'toolbar',
      size: 'kira',
    },
  },
);
export type ButtonVariants = VariantProps<typeof buttonVariants>;
