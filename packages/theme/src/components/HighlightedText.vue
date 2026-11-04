<script setup lang="ts">
// Text with its matches marked. The caller splits the text into `parts` (workbench `splitMatches`);
// `current` is the index in `parts` of the active match, which gets the stronger colour.
defineProps<{
  parts: readonly { text: string; match: boolean }[];
  current?: number;
}>();
</script>

<template>
  <span data-slot="highlighted-text"
    ><template v-for="(part, i) in parts" :key="i"
      ><mark
        v-if="part.match"
        data-testid="match"
        :class="[
          'rounded-kira-sm text-inherit',
          i === current ? 'bg-search-match-current' : 'bg-search-match',
        ]"
        >{{
          part.text
        }}</mark
      ><template v-else>{{ part.text }}</template></template
    ></span
  >
</template>
