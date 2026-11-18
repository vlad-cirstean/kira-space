<script setup lang="ts">
import type { ImportFile } from '@shared/domain/memoryImport';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';
import { FILE_KIND_LABEL, FILE_STATE_LABEL } from './importFormat';

// P211: one file of an import — fixed height, the virtualized list's row.
const props = defineProps<{ file: ImportFile; selected: boolean }>();
const emit = defineEmits<{ select: []; retry: [] }>();

const variant = computed(() => {
  switch (props.file.state) {
    case 'done':
      return 'ok';
    case 'failed':
      return 'error';
    case 'skipped':
    case 'cancelled':
      return 'default';
    default:
      return 'info';
  }
});
</script>

<template>
  <div
    :class="cn(rowVariants({ layout: 'tree', selected }), 'h-full gap-1.5 pr-3')"
    :data-testid="`import-file-${file.relPath}`"
    :data-state="file.state"
  >
    <button
      type="button"
      class="flex h-full min-w-0 flex-1 cursor-default items-center gap-1.5 border-0 bg-transparent pl-3 text-left"
      @click="emit('select')"
    >
      <span class="min-w-0 flex-1"><TruncatedText :text="file.relPath" /></span>
      <span :class="[rowMeta, 'shrink-0']" data-testid="import-file-kind">{{
        t(FILE_KIND_LABEL[file.kind])
      }}</span>
      <span
        v-if="file.verified > 0"
        :class="[rowMeta, 'shrink-0']"
        data-testid="import-file-verified"
        >{{
          t('memory.import.file.verified.label', { count: file.verified })
        }}</span
      >
      <span v-if="file.state === 'done'" :class="[rowMeta, 'shrink-0']">
        +{{ file.added }}
        ~{{ file.updated }}
        ={{ file.noop }}
      </span>
      <span v-else-if="file.chunkCount > 0" class="shrink-0 text-kira-sm text-muted-foreground">
        {{ file.chunksDone }}
        / {{ file.chunkCount }}
      </span>
      <Badge :variant="variant">{{ t(FILE_STATE_LABEL[file.state]) }}</Badge>
    </button>
    <Button
      v-if="file.state === 'failed'"
      size="kira"
      variant="toolbar"
      :data-testid="`import-retry-file-${file.relPath}`"
      @click="emit('retry')"
      ><CodiconIcon :name="ICON.refresh" :size="13" />
      {{ t('common.action.retry.label') }}
    </Button>
  </div>
</template>
