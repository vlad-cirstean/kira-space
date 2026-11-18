<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
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
  <Collapsible v-model:open="open" :data-testid="testid">
    <KeyValueList>
      <div
        v-for="(row, i) in shown"
        :key="`${i}-${row.name}`"
        class="contents"
        :data-testid="rowTestid?.(row.name)"
      >
        <dt class="font-data">{{ row.name }}</dt>
        <dd class="break-all font-data">
          {{ row.value }}
          <span v-if="row.secret" class="text-subtle">{{ t('common.secret.label') }}</span>
        </dd>
      </div>
    </KeyValueList>
    <CollapsibleTrigger v-if="hidden > 0 && !open" as-child>
      <Button
        variant="link"
        size="inline"
        class="text-kira-sm font-normal text-muted-foreground underline decoration-border underline-offset-3"
        data-testid="approval-more"
      >
        {{ t('common.more.label', { count: hidden }) }}
      </Button>
    </CollapsibleTrigger>
  </Collapsible>
</template>
