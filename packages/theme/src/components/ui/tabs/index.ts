import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Tabs } from '@theme/components/ui/tabs/Tabs.vue';
export { default as TabsContent } from '@theme/components/ui/tabs/TabsContent.vue';
export { default as TabsList } from '@theme/components/ui/tabs/TabsList.vue';
export { default as TabsTrigger } from '@theme/components/ui/tabs/TabsTrigger.vue';

// The one tab-chip class definition — TabStrip, ConnectionDialog's detail tabs,
// ConsoleView's result tabs and TitleBar's mode tabs all render through this.
export const tabChipVariants = cva(
  'inline-flex items-center gap-1 h-control-lg rounded-kira-sm border cursor-default shrink-0 max-w-52 text-kira-md',
  {
    variants: {
      active: {
        true: 'is-active bg-elevated border-transparent text-fg',
        false: 'border-transparent',
      },
      // Multi-select (P727): grey selection tokens, never the accent (N18).
      selected: { true: 'is-selected', false: '' },
      size: { default: 'px-1.5', icon: 'px-1', wide: 'px-3' },
    },
    compoundVariants: [
      { active: false, selected: false, class: 'text-muted-foreground hover:bg-field' },
      { active: false, selected: true, class: 'bg-select text-fg' },
      { active: true, selected: true, class: 'ring-1 ring-inset ring-select-edge' },
    ],
    defaultVariants: { active: false, selected: false, size: 'default' },
  },
);

export type TabChipVariants = VariantProps<typeof tabChipVariants>;
