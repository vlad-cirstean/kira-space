<script setup lang="ts">
import StatusDot from '@theme/components/StatusDot.vue';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import AdeTaskUpdates from './AdeTaskUpdates.vue';

// The "updated" dot on a task row; click lists what changed with Mark seen. The popover closes
// itself when the list empties: the card unmounts this trigger then.
defineProps<{ taskId: string }>();
</script>

<template>
  <Popover>
    <PopoverTrigger as-child>
      <Button
        type="button"
        variant="toolbar"
        size="inline"
        class="size-4.5"
        :aria-label="t('space.ade.updates.dot.aria')"
        data-testid="ade-task-updated"
        @click.stop
      >
        <StatusDot tone="info" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="start" flush class="w-80" @click.stop>
      <AdeTaskUpdates :task-id="taskId" :framed="false" />
    </PopoverContent>
  </Popover>
</template>
