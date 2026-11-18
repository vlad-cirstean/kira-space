<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import DetailSection from '@theme/components/DetailSection.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { FieldDescription } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { useLeases } from '../leases/queries';
import { useAdeLeasesStore } from '../state/adeLeases';

// Every lease queue: its holder (menu: request or force release) and the waiters in grant order.
const store = useAdeLeasesStore();
const { data } = useLeases();

type Queue = NonNullable<typeof data.value>['queues'][number];
function holderItems(q: Queue): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'request',
      testId: 'ade-lease-request',
      label: t('space.ade.leases.requestRelease.label'),
      icon: 'bell',
      disabled: !!q.holder?.releaseAsked || store.leaseBusy(q.name),
      run: () => store.requestRelease(q.name),
    },
    {
      type: 'item',
      id: 'force',
      testId: 'ade-lease-force',
      label: t('space.ade.leases.forceRelease.label'),
      icon: 'debug-disconnect',
      danger: true,
      disabled: store.leaseBusy(q.name),
      run: () => store.forceRelease(q),
    },
  ];
}
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto p-3" data-testid="ade-leases">
    <div class="flex max-w-3xl flex-col gap-3">
      <form class="flex gap-2" @submit.prevent="store.addQueue()">
        <Input
          v-model="store.draft"
          :placeholder="t('space.ade.leases.add.placeholder')"
          :aria-label="t('space.ade.leases.add.label')"
          data-testid="ade-lease-queue-input"
        />
        <Button
          type="submit"
          variant="dialog-primary"
          size="kira"
          :disabled="!store.canAdd()"
          data-testid="ade-lease-queue-add"
        >
          {{ t('space.ade.leases.add.label') }}
        </Button>
      </form>
      <FieldDescription v-if="!data?.queues.length" data-testid="ade-leases-empty">
        {{ t('space.ade.leases.empty.hint') }}
      </FieldDescription>
      <DetailSection
        v-for="q in data?.queues ?? []"
        :key="q.name"
        :title="q.name"
        :data-testid="`ade-lease-queue-${q.name}`"
      >
        <template #actions>
          <Button
            variant="toolbar"
            size="kira"
            data-tone="error"
            :aria-label="t('space.ade.leases.removeQueue.label')"
            :pending="store.saving()"
            :data-testid="`ade-lease-remove-${q.name}`"
            @click="store.removeQueue(q.name)"
            ><CodiconIcon :name="ICON.delete" :size="13" />
            {{ t('space.ade.leases.removeQueue.label') }}
          </Button>
        </template>
        <KeyValueList>
          <dt>{{ t('space.ade.leases.holder.label') }}</dt>
          <dd class="flex min-w-0 items-center gap-2">
            <DropdownMenu v-if="q.holder">
              <DropdownMenuTrigger as-child>
                <Button
                  variant="toolbar"
                  size="kira"
                  class="min-w-0 truncate"
                  :pending="store.leaseBusy(q.name)"
                  :data-testid="`ade-lease-holder-${q.name}`"
                >
                  {{ q.holder.taskTitle }}
                  <CodiconIcon name="chevron-down" :size="13" data-icon="inline-end" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <MenuItems :items="holderItems(q)" />
              </DropdownMenuContent>
            </DropdownMenu>
            <span v-else class="text-muted-foreground" :data-testid="`ade-lease-free-${q.name}`">{{
              t('space.ade.leases.free.label')
            }}</span>
            <Badge v-if="q.holder?.releaseAsked" variant="warn">{{
              t('space.ade.leases.asked.label')
            }}</Badge>
          </dd>
          <dt>{{ t('space.ade.leases.waiters.label') }}</dt>
          <dd>
            <ol v-if="q.waiters.length" class="m-0 flex min-w-0 flex-col gap-1 p-0">
              <li
                v-for="w in q.waiters"
                :key="w.sessionId"
                class="flex list-none items-center gap-2"
                :data-testid="`ade-lease-waiter-${q.name}-${w.position}`"
              >
                <span class="text-muted-foreground">#{{ w.position }}</span>
                <span class="min-w-0 truncate">{{ w.taskTitle }}</span>
              </li>
            </ol>
            <span v-else class="text-muted-foreground">{{
              t('space.ade.leases.waiters.empty.hint')
            }}</span>
          </dd>
        </KeyValueList>
      </DetailSection>
    </div>
  </div>
</template>
