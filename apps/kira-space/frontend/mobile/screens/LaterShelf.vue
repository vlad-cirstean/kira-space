<script setup lang="ts">
import { useHostLabels } from '@ade/hostLabels';
import type { Shelf, Task } from '@ade/wire';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import { moveId, useSortableReorder } from '@workbench/behaviour/sortableReorder';
import { errorText } from '@workbench/bridge/codedError';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import { computed, ref, useTemplateRef } from 'vue';
import { newIntentKey, useAdeWrites } from '../state/useAdeWrites';

// One shelf of the Later screen: drag a row by its handle to reorder or to drop it on the other
// shelf, tap a row for its notes. Both shelves share a Sortable group.
const props = defineProps<{ shelf: Shelf; tasks: readonly Task[]; canWrite: boolean }>();
const emit = defineEmits<{ error: [message: string] }>();

const hostLabels = useHostLabels();
const writes = useAdeWrites();
const open = ref<string | null>(null);
const list = useTemplateRef<HTMLElement>('list');
const ids = computed(() => props.tasks.map((task) => task.id));

function facts(task: Task): string {
  const link = task.hostLink;
  return [task.jira?.key ?? '', link ? hostLabels.factOf(link) : ''].filter(Boolean).join(' · ');
}

async function put(taskId: string, beforeId: string): Promise<void> {
  emit('error', '');
  try {
    await writes.shelveTask.mutateAsync({
      taskId,
      shelf: props.shelf,
      beforeId,
      key: newIntentKey(),
    });
  } catch (err) {
    emit('error', errorText(err));
  }
}

useSortableReorder(
  list,
  () => ids.value,
  (fromId, toId) => {
    const next = moveId(ids.value, fromId, toId);
    void put(fromId, next[next.indexOf(fromId) + 1] ?? '');
  },
  {
    draggable: '[data-shelf-row]',
    handle: '[data-drag-handle]',
    direction: 'vertical',
    disabled: () => !props.canWrite,
    group: 'later-shelf',
    itemId: (el) => el.dataset.taskId ?? null,
    onAdd: (id, beforeId) => void put(id, beforeId ?? ''),
  },
);
</script>

<template>
  <section :data-testid="`later-shelf-${shelf}`">
    <SectionHeading
      :label="shelf === 'later' ? t('common.later.label') : t('space.common.backlog.label')"
    />
    <ol ref="list" class="m-0 flex min-h-12 list-none flex-col p-0">
      <li
        v-for="task in tasks"
        :key="task.id"
        class="border-b border-border bg-bg"
        :data-task-id="task.id"
        data-shelf-row
        data-testid="later-item"
      >
        <div class="flex items-stretch">
          <button
            type="button"
            class="flex min-w-0 flex-1 items-start gap-3 bg-transparent px-3 py-3 text-left text-fg"
            :aria-expanded="open === task.id"
            @click="open = open === task.id ? null : task.id"
          >
            <span class="flex min-w-0 flex-1 flex-col gap-0.5">
              <span class="text-kira-md">{{ task.title }}</span>
              <span class="text-kira-sm text-subtle">
                <template v-if="facts(task)">{{ facts(task) }} · </template>
                <RelativeTime :at="task.createdAt" />
              </span>
            </span>
          </button>
          <span
            v-if="canWrite"
            class="flex size-touch shrink-0 touch-none items-center justify-center text-subtle"
            role="img"
            :aria-label="t('mobile.backlog.handle.aria')"
            data-drag-handle
            data-testid="later-handle"
          >
            <CodiconIcon name="gripper" :size="16" />
          </span>
        </div>
        <p
          v-if="open === task.id"
          class="m-0 whitespace-pre-wrap px-3 pb-3 text-kira-md text-muted-foreground"
          data-testid="later-notes"
        >
          {{ task.notes || t('mobile.later.notes.empty') }}
        </p>
      </li>
      <li v-if="!tasks.length" class="px-3 py-4 text-kira-md text-subtle" data-testid="later-empty">
        {{
          shelf === 'later'
            ? t('mobile.later.laterEmpty.empty')
            : t('mobile.later.backlogEmpty.empty')
        }}
      </li>
    </ol>
  </section>
</template>
