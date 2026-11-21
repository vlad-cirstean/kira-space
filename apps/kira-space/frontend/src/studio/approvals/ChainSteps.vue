<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// The chain this request belongs to. Earlier steps are known only through the kept variables it
// reads (each names the request that kept it); later steps do not exist yet.
const props = defineProps<{
  kept: readonly KeptUse[];
  /** This step: method and URL, or the statement's first line. */
  current: string;
  scripts: readonly ('prepare' | 'transform')[];
}>();

const SCRIPT_LABEL = {
  prepare: 'studio.dbMcp.approval.chain.prepare.label',
  transform: 'studio.dbMcp.approval.chain.transform.label',
} as const;

const earlier = computed(() => {
  const seen = new Map<
    string,
    { request: string; tool: string; byScript: boolean; names: string[] }
  >();
  for (const u of props.kept) {
    if (!u.source?.request) continue;
    const row = seen.get(u.source.request) ?? {
      request: u.source.request,
      tool: u.source.tool,
      byScript: u.source.kind === 'script',
      names: [],
    };
    row.names.push(u.name);
    seen.set(u.source.request, row);
  }
  return [...seen.values()];
});
</script>

<template>
  <ol
    v-if="earlier.length"
    class="m-0 flex list-none flex-col gap-1 p-0 text-kira-sm"
    data-testid="approval-chain"
  >
    <li
      v-for="step in earlier"
      :key="step.request"
      class="flex min-w-0 items-center gap-2 text-muted-foreground"
      data-testid="approval-chain-step"
    >
      <CodiconIcon name="pass" :size="13" class="shrink-0 text-ok" />
      <span class="shrink-0">{{ t('studio.dbMcp.approval.chain.earlier.label') }}</span>
      <span class="min-w-0 truncate font-data">{{ step.request }}</span>
      <span class="shrink-0">{{
        t(
          step.byScript
            ? 'studio.dbMcp.approval.chain.keptByScript.status'
            : 'studio.dbMcp.approval.chain.kept.status',
          { names: step.names.join(', ') },
        )
      }}</span>
    </li>
    <li class="flex min-w-0 items-center gap-2 text-fg" data-testid="approval-chain-current">
      <CodiconIcon name="arrow-right" :size="13" class="shrink-0" />
      <span class="shrink-0 font-medium">{{ t('studio.dbMcp.approval.chain.current.label') }}</span>
      <span class="min-w-0 truncate font-data">{{ current }}</span>
      <Badge v-for="s in scripts" :key="s" variant="warn" class="gap-1">
        <CodiconIcon name="code" :size="12" />{{ t(SCRIPT_LABEL[s]) }}
      </Badge>
    </li>
  </ol>
</template>
