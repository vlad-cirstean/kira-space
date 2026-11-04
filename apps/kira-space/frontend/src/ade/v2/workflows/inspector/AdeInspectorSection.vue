<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Collapsible, CollapsibleContent } from '@theme/components/ui/collapsible';
import { usePersistedState } from '@workbench/behaviour/persisted';

const props = defineProps<{ title: string; name: string; collapsible?: boolean }>();
const open = usePersistedState(`ade.wf.inspector.${props.name}`, false);
</script>

<template>
  <Collapsible
    v-if="collapsible"
    v-model:open="open"
    class="flex flex-col gap-3"
    :data-testid="`ade-wf-step-section-${name}`"
  >
    <SectionHeading
      collapsible
      :label="title"
      :expanded="open"
      :data-testid="`ade-wf-step-${name}`"
      @toggle="open = !open"
    />
    <CollapsibleContent class="flex flex-col gap-3">
      <slot />
    </CollapsibleContent>
  </Collapsible>
  <section v-else class="flex flex-col gap-3" :data-testid="`ade-wf-step-section-${name}`">
    <SectionHeading :label="title" />
    <slot />
  </section>
</template>
