<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { FieldDescription } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import AdeChip from '../AdeChip.vue';
import { useLeases } from '../leases/queries';
import { useAdeLeasesStore } from '../state/adeLeases';

// Every lease queue: its holder (menu: request or force release) and the waiters in grant order.
const store = useAdeLeasesStore();
const { data } = useLeases();
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto p-4" data-testid="ade-leases">
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
      <Card
        v-for="q in data?.queues ?? []"
        :key="q.name"
        variant="section"
        class="flex flex-col gap-2 p-3"
        :data-testid="`ade-lease-queue-${q.name}`"
      >
        <div class="flex items-center gap-2">
          <span class="min-w-0 flex-1 truncate font-medium">{{ q.name }}</span>
          <Button
            variant="toolbar"
            size="kira"
            :aria-label="t('space.ade.leases.removeQueue.label')"
            :data-testid="`ade-lease-remove-${q.name}`"
            @click="store.removeQueue(q.name)"
          >
            {{ t('space.ade.leases.removeQueue.label') }}
          </Button>
        </div>
        <div class="flex items-center gap-2 text-kira-sm">
          <span class="w-16 shrink-0 text-muted-foreground">{{
            t('space.ade.leases.holder.label')
          }}</span>
          <DropdownMenu v-if="q.holder">
            <DropdownMenuTrigger as-child>
              <Button
                variant="toolbar"
                size="kira"
                class="min-w-0 truncate"
                :data-testid="`ade-lease-holder-${q.name}`"
              >
                {{ q.holder.taskTitle }}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuItem
                :disabled="q.holder.releaseAsked"
                data-testid="ade-lease-request"
                @select="store.requestRelease(q.name)"
              >
                {{ t('space.ade.leases.requestRelease.label') }}
              </DropdownMenuItem>
              <DropdownMenuItem data-testid="ade-lease-force" @select="store.forceRelease(q)">
                {{ t('space.ade.leases.forceRelease.label') }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <span v-else class="text-muted-foreground" :data-testid="`ade-lease-free-${q.name}`">{{
            t('space.ade.leases.free.label')
          }}</span>
          <AdeChip
            v-if="q.holder?.releaseAsked"
            :label="t('space.ade.leases.asked.label')"
            tone="amber"
          />
        </div>
        <div class="flex items-start gap-2 text-kira-sm">
          <span class="w-16 shrink-0 text-muted-foreground">{{
            t('space.ade.leases.waiters.label')
          }}</span>
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
        </div>
      </Card>
    </div>
  </div>
</template>
