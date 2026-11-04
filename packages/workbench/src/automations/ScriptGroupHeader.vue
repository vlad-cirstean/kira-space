<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import InlineEditInput from '@workbench/components/InlineEditInput.vue';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';

defineProps<{ name: string; count: number; open: boolean; renaming: boolean }>();
const emit = defineEmits<{
  toggle: [];
  commit: [name: string];
  cancel: [];
  menu: [event: MouseEvent];
}>();
</script>

<template>
  <!-- biome-ignore lint/a11y/noStaticElementInteractions: role and tabindex arrive in $attrs from useListNavigation.rowProps. -->
  <div
    :class="rowVariants({ layout: 'tree' })"
    :style="rowIndent(0)"
    data-testid="script-group"
    :data-name="name"
    @contextmenu.prevent.stop="emit('menu', $event)"
  >
    <TreeTwisty :expanded="open" :has-children="count > 0" @toggle="emit('toggle')" />
    <CodiconIcon name="folder-library" :size="13" class="shrink-0 text-muted-foreground" />
    <InlineEditInput
      v-if="renaming"
      :name="name"
      data-testid="script-collection-rename-input"
      @commit="(next) => emit('commit', next)"
      @cancel="emit('cancel')"
    />
    <button
      v-else
      type="button"
      class="flex-1 min-w-0 cursor-default border-0 bg-transparent p-0 text-left font-medium text-inherit"
      :aria-expanded="open"
      @click="emit('toggle')"
      ><TruncatedText :text="name" /></button
    >
    <span :class="rowMeta">{{ count }}</span>
  </div>
</template>
