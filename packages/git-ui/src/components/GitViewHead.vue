<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import ViewHead from '@workbench/components/ViewHead.vue';
import { computed } from 'vue';

export interface RepoHead {
  readonly name: string;
  readonly dir?: string;
  readonly color?: string;
}

// A repo's view head: colour dot (none for no colour), icon, name with dim path prefix, badges.
const props = defineProps<{
  repo: RepoHead;
  headLabel?: string;
  ahead?: number;
  behind?: number;
  operation?: string;
}>();

const hasColor = computed(() => props.repo.color !== undefined && props.repo.color !== 'none');
const sync = computed(() => {
  const parts: string[] = [];
  if (props.ahead) parts.push(`↑${props.ahead}`);
  if (props.behind) parts.push(`↓${props.behind}`);
  return parts.join(' ');
});
</script>

<template>
  <ViewHead
    testid="git-view-head"
    :color="hasColor ? repo.color : undefined"
    icon="source-control"
    :prefix="repo.dir"
    :title="repo.name"
    title-testid="git-view-head-name"
  >
    <Badge v-if="headLabel" data-testid="git-view-head-branch">{{ headLabel }}</Badge>
    <Badge v-if="sync" data-testid="git-view-head-sync">{{ sync }}</Badge>
    <template v-if="operation" #actions>
      <Badge variant="warn" data-testid="git-view-head-operation">{{ operation }}</Badge>
    </template>
  </ViewHead>
</template>
