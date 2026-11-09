<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { Collapsible, CollapsibleTrigger } from '@theme/components/ui/collapsible';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';

// A name/value list in the data font, values exactly as sent. Over three rows the rest hides behind "+N more".
const props = defineProps<{
  rows: { name: string; value: string; secret?: boolean }[];
  testid: string;
  rowTestid?: (name: string) => string;
}>();

const VISIBLE = 3;
const open = ref(false);
const shown = computed(() => (open.value ? props.rows : props.rows.slice(0, VISIBLE)));
const hidden = computed(() => Math.max(0, props.rows.length - VISIBLE));
</script>

<template>
  <Collapsible
    v-model:open="open"
    class="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-0.5 font-data text-kira-md leading-normal"
    :data-testid="testid"
  >
    <div
      v-for="(row, i) in shown"
      :key="`${i}-${row.name}`"
      class="contents"
      :data-testid="rowTestid?.(row.name)"
    >
      <span class="text-muted-foreground">{{ row.name }}</span>
      <span class="break-all">
        {{ row.value }}
        <span v-if="row.secret" class="text-subtle">{{ t('common.secret.label') }}</span>
      </span>
    </div>
    <CollapsibleTrigger v-if="hidden > 0 && !open" as-child>
      <Button
        variant="link"
        size="inline"
        class="col-span-full justify-self-start text-kira-sm font-normal text-muted-foreground underline decoration-border underline-offset-3"
        data-testid="approval-more"
      >
        {{ t('common.more.label', { count: hidden }) }}
      </Button>
    </CollapsibleTrigger>
  </Collapsible>
</template>
