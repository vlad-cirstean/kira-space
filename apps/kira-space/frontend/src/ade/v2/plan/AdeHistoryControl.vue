<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import AdeTip from '../AdeTip.vue';
import AdeArchivedFinder from './AdeArchivedFinder.vue';

// One compact row above the first day: toggles the loaded history and finds any archived task.
defineProps<{ count: number; tip: string; open: boolean }>();
const emit = defineEmits<{ load: []; hide: []; pick: [taskId: string] }>();
</script>

<template>
  <div class="mb-1 flex h-row items-center gap-1">
    <AdeTip :text="tip">
      <Button
        variant="toolbar"
        size="kira"
        class="h-row gap-1 px-1.5 text-kira-sm text-muted-foreground"
        :data-open="open || undefined"
        data-testid="ade-load-history"
        @click="open ? emit('hide') : emit('load')"
      >
        <CodiconIcon name="history" :size="12" />
        <span>{{
          open ? t('space.ade.plan.history.hide.label') : t('common.module.history.label')
        }}</span>
        <span v-if="!open && count > 0" class="tabular-nums text-subtle">{{ count }}</span>
      </Button>
    </AdeTip>
    <AdeArchivedFinder @pick="(id) => emit('pick', id)" />
  </div>
</template>
