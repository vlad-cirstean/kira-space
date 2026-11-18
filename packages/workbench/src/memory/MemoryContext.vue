<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';
import { GROUP_KIND_LABEL } from './labels';
import { useMemoryContext } from './queries';

// P272: facts stored alongside the selected one, per group (session, request, task, document, thread).
const props = defineProps<{ memoryId: string }>();
const emit = defineEmits<{ select: [id: string] }>();
const context = useMemoryContext(() => props.memoryId);
const groups = computed(() => context.data.value?.groups ?? []);
</script>

<template>
  <div v-if="groups.length" class="flex flex-col gap-2" data-testid="memory-context">
    <SectionHeading :label="t('memory.start.context.label')" />
    <section
      v-for="group in groups"
      :key="group.id"
      class="flex flex-col gap-0.5"
      :data-kind="group.kind"
      data-testid="memory-context-group"
    >
      <div class="flex min-w-0 items-baseline gap-1.5 px-1.5 text-kira-sm text-muted-foreground">
        <span class="shrink-0 font-medium">{{ t(GROUP_KIND_LABEL[group.kind]) }}</span>
        <TruncatedText class="font-data" :text="group.title || group.key" />
      </div>
      <button
        v-for="fact in group.facts"
        :key="fact.id"
        type="button"
        :class="
          cn(
            rowVariants({ layout: 'menu', muted: fact.historical }),
            'h-auto w-full whitespace-normal py-1 text-left',
          )
        "
        :data-testid="`memory-context-fact-${fact.id}`"
        @click="emit('select', fact.id)"
      >
        {{ fact.fact }}
      </button>
      <span
        v-if="group.total > group.facts.length"
        class="px-1.5 text-kira-sm text-muted-foreground"
        >{{
          t('memory.start.context.more.label', { count: group.total - group.facts.length })
        }}</span
      >
    </section>
  </div>
</template>
