<script setup lang="ts">
import DetailHeader from '@theme/components/DetailHeader.vue';
import { t } from '@theme/i18n';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import { computed } from 'vue';
import { branchGroups, branchOptions } from '../../lib/refOptions.ts';
import type { RefsState } from '../../state/refs.ts';

const props = defineProps<{ refsState: RefsState }>();
const emit = defineEmits<(e: 'pick', name: string) => void>();

const branches = computed(() => branchOptions(props.refsState, { remote: true }));
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="review-no-branch">
    <DetailHeader :title="t('git.review.start.title')">
      <template #description>{{ t('git.review.noBranch.emptyHint') }}</template>
    </DetailHeader>
    <div class="flex min-h-0 flex-1 flex-col px-3 pb-3">
      <BranchPicker
        inline
        class="min-h-0 flex-1"
        testid="review-branch"
        :branches="branches"
        :groups="branchGroups()"
        :placeholder="t('common.action.choose.label')"
        :loading="refsState.head.value === undefined"
        @select="(o) => emit('pick', o.value)"
      />
    </div>
  </div>
</template>
