<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// The disclosure-triangle button shared by TreeRow.vue, CollectionRow.vue,
// AdeFileTreeRow.vue and GitPanel.vue's own repo-row expand -- byte-identical utility sets, once the
// retired `.twisty` `@utility` (base.css) is factored out into a real component instead of a
// class name. `hasChildren` both hides the button (`invisible`, same as the class it replaces —
// still occupies its layout slot, so sibling content never reflows) and gates the emit, matching
// every real consumer's own pre-existing "only emit while there is something to expand" guard.
// Click always stops propagation -- every consumer's own pre-existing behaviour (either via a
// `.stop` modifier or an explicit `e.stopPropagation()` in its own handler), never something a
// twisty click should leak to the row underneath it.
//
// `ariaExpanded`/`testid` are optional escape hatches for the one consumer (GitPanel.vue's
// worktree-expand button) whose own pre-existing markup carried a real `aria-expanded` and a
// distinct `data-testid="repo-row-expand"` (real-test-dependent, `repo-workspace.spec.ts`) rather
// than this component's own default `tree-twisty` -- both are preserved exactly, not dropped.
const props = withDefaults(
  defineProps<{
    expanded: boolean;
    hasChildren: boolean;
    ariaExpanded?: boolean;
    testid?: string;
    class?: HTMLAttributes['class'];
  }>(),
  { testid: 'tree-twisty' },
);
const emit = defineEmits<{ toggle: [] }>();

function onClick(e: MouseEvent): void {
  e.stopPropagation();
  if (props.hasChildren) emit('toggle');
}
</script>

<template>
  <button
    type="button"
    tabindex="-1"
    :class="
      cn(
        'flex size-3.5 shrink-0 items-center justify-center border-0 bg-transparent p-0 text-muted-foreground',
        !hasChildren && 'invisible',
        props.class,
      )
    "
    :aria-label="expanded ? t('common.action.collapse.label') : t('common.action.expand.label')"
    :aria-expanded="ariaExpanded"
    :data-testid="testid"
    @click="onClick"
  >
    <slot>
      <CodiconIcon :name="expanded ? 'chevron-down' : 'chevron-right'" :size="13" />
    </slot>
  </button>
</template>
