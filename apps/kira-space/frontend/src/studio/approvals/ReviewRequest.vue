<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import { ref } from 'vue';

// The request a held result answers, collapsed above the result: the review is about the result, and the
// user already approved this request.
defineProps<{ testid: string }>();

const open = ref(false);
</script>

<template>
  <ApprovalBox :testid="testid">
    <Collapsible v-model:open="open" class="flex flex-col gap-2">
      <CollapsibleTrigger as-child>
        <Button
          variant="link-meta"
          size="inline"
          class="self-start"
          :data-testid="`${testid}-toggle`"
        >
          {{ t('common.field.request.label') }}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent :data-testid="`${testid}-content`">
        <slot />
      </CollapsibleContent>
    </Collapsible>
  </ApprovalBox>
</template>
