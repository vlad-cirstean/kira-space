<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Collapsible, CollapsibleContent } from '@theme/components/ui/collapsible';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import { ref } from 'vue';

// The request a held result answers, collapsed below the result: the review is about the result, and the
// user already approved this request.
const props = defineProps<{ testid: string }>();

const open = ref(false);
</script>

<template>
  <ApprovalBox :testid="testid">
    <Collapsible v-model:open="open" class="flex flex-col gap-2">
      <SectionHeading
        flush
        collapsible
        :expanded="open"
        :label="t('common.field.request.label')"
        :data-testid="`${props.testid}-toggle`"
        @toggle="open = !open"
      />
      <CollapsibleContent class="flex flex-col gap-2" :data-testid="`${testid}-content`">
        <slot />
      </CollapsibleContent>
    </Collapsible>
  </ApprovalBox>
</template>
