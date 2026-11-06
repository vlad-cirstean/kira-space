<script setup lang="ts">
import { computed } from 'vue';

// Sample rows at the chosen density; fake data, not wording.
const props = defineProps<{ heightPx: number }>();
const height = computed(() => props.heightPx);
const PREVIEW = {
  idColumn: 'id',
  emailColumn: 'email',
  rows: [
    { id: 'c1d0-88ae', email: 'rowan.brooks@example.com' },
    { id: '7f2b-19cd', email: 'amari.osei@example.com' },
  ],
};
</script>

<template>
  <!-- Every cell below carries the full computed style of the old shared
       .row-preview-cell/.row-preview-gutter/.row-preview-grow/.row-preview-head rules, resolved
       per-instance rather than stacking conflicting same-property utilities -- the head
       row's `.row-preview-head .row-preview-cell` descendant selector outranked the gutter/grow
       siblings' own color/font-size on specificity alone, and .row-preview-grow's flex-1 always
       beat .row-preview-cell's own flex-basis by source order. That descendant selector's own
       `font-family: inherit` is dropped rather than ported: none of these 3 header cells carry
       font-data (unlike their data-row counterparts below), so they already inherit naturally
       -- porting `inherit` as a class would re-state the default. -->
  <div class="overflow-hidden rounded-kira">
    <div class="flex bg-elevated border-b border-border">
      <span
        class="flex items-center truncate px-2 border-r border-border flex-none basis-9 justify-end text-muted-foreground text-kira-sm bg-elevated"
        :style="{ height: `${height}px` }"
      />
      <span
        class="flex items-center truncate px-2 border-r border-border flex-none basis-37.5 text-muted-foreground text-kira-sm"
        :style="{ height: `${height}px` }"
        >{{
          PREVIEW.idColumn
        }}</span
      >
      <span
        class="flex items-center truncate px-2 border-r border-border flex-1 text-muted-foreground text-kira-sm"
        :style="{ height: `${height}px` }"
        >{{
          PREVIEW.emailColumn
        }}</span
      >
    </div>
    <div class="flex" :style="{ height: `${height}px` }">
      <span
        class="flex items-center truncate px-2 border-r border-border flex-none basis-9 justify-end text-subtle text-kira-sm bg-elevated font-data"
        >1</span
      >
      <span
        class="flex items-center truncate px-2 border-r border-border flex-none basis-37.5 text-fg text-kira-md font-data"
        >{{
          PREVIEW.rows[0].id
        }}</span
      >
      <span
        class="flex items-center truncate px-2 border-r border-border flex-1 text-fg text-kira-md font-data"
        >{{
          PREVIEW.rows[0].email
        }}</span
      >
    </div>
    <div class="flex border-t border-border" :style="{ height: `${height}px` }">
      <span
        class="flex items-center truncate px-2 border-r border-border flex-none basis-9 justify-end text-subtle text-kira-sm bg-elevated font-data"
        >2</span
      >
      <span
        class="flex items-center truncate px-2 border-r border-border flex-none basis-37.5 text-fg text-kira-md font-data"
        >{{
          PREVIEW.rows[1].id
        }}</span
      >
      <span
        class="flex items-center truncate px-2 border-r border-border flex-1 text-fg text-kira-md font-data"
        >{{
          PREVIEW.rows[1].email
        }}</span
      >
    </div>
  </div>
</template>
