<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import RestoreRowItem from './RestoreRowItem.vue';
import type { RestoreRow } from './types';

// Presentational: the adapters (Docker, Runs) own the data, the calls and the placement.
const props = defineProps<{
  title: string;
  description: string;
  /** Codicon name. */
  icon: string;
  rows: RestoreRow[];
  pending: boolean;
  dismissing: boolean;
  callError?: string;
}>();
/** Ids the person unchecked. A ready row counts as selected unless listed, so rows that arrive later start checked. */
const excluded = defineModel<string[]>('excluded', { default: () => [] });
const emit = defineEmits<{ confirm: [ids: string[]]; dismiss: []; close: [] }>();

const listed = computed(() => props.rows.filter((r) => r.state !== 'unavailable'));
const unavailable = computed(() => props.rows.filter((r) => r.state === 'unavailable'));
const ready = computed(() => props.rows.filter((r) => r.state === 'ready'));
const chosen = computed(() =>
  ready.value.filter((r) => !excluded.value.includes(r.id)).map((r) => r.id),
);
const showUnavailable = ref(false);

const isChecked = (r: RestoreRow) => !excluded.value.includes(r.id);

function toggle(id: string, on: boolean): void {
  const rest = excluded.value.filter((x) => x !== id);
  excluded.value = on ? rest : [...rest, id];
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent size="lg" data-testid="restore-dialog">
      <DialogHeader>
        <span
          class="grid size-7 shrink-0 place-items-center rounded-kira bg-field text-muted-foreground"
        >
          <CodiconIcon :name="icon" :size="16" />
        </span>
        <div class="flex min-w-0 flex-col gap-0.5">
          <DialogTitle>{{ title }}</DialogTitle>
          <DialogDescription class="text-kira-sm text-muted-foreground">{{
            description
          }}</DialogDescription>
        </div>
      </DialogHeader>

      <DialogBody>
        <div v-if="ready.length" class="flex items-center justify-between gap-2">
          <span class="text-kira-sm text-muted-foreground" data-testid="restore-selection">{{
            t('workbench.restore.selection.status', {
              selected: chosen.length,
              total: ready.length,
            })
          }}</span>
          <div class="flex items-center gap-1">
            <Button
              variant="toolbar"
              size="kira"
              :disabled="chosen.length === ready.length"
              data-testid="restore-select-all"
              @click="excluded = []"
              >{{
                t('common.action.selectAll.label')
              }}</Button
            >
            <Button
              variant="toolbar"
              size="kira"
              :disabled="chosen.length === 0"
              data-testid="restore-select-none"
              @click="excluded = ready.map((r) => r.id)"
              >{{
                t('common.action.selectNone.label')
              }}</Button
            >
          </div>
        </div>

        <ul v-if="listed.length" class="m-0 flex list-none flex-col rounded-kira-sm">
          <li
            v-for="r in listed"
            :key="r.id"
            data-testid="restore-row"
            :data-id="r.id"
            :data-state="r.state"
            :data-status="r.status"
          >
            <RestoreRowItem :row="r" :checked="isChecked(r)" @toggle="(on) => toggle(r.id, on)" />
          </li>
        </ul>

        <Collapsible
          v-if="unavailable.length"
          v-model:open="showUnavailable"
          data-testid="restore-unavailable"
        >
          <CollapsibleTrigger as-child>
            <Button
              variant="toolbar"
              size="kira"
              class="w-full justify-start"
              data-testid="restore-unavailable-toggle"
            >
              <CodiconIcon :name="showUnavailable ? 'chevron-down' : 'chevron-right'" :size="13" />
              {{ t('workbench.restore.unavailable.title', { count: unavailable.length }) }}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul class="m-0 mt-1 flex list-none flex-col gap-1 p-0 pl-6 text-kira-sm">
              <li
                v-for="r in unavailable"
                :key="r.id"
                class="flex min-w-0 flex-col"
                data-testid="restore-row"
                :data-id="r.id"
                data-state="unavailable"
                :data-status="r.status"
              >
                <span class="truncate">{{ r.label }}</span>
                <span v-if="r.reason" class="text-muted-foreground">{{ r.reason }}</span>
                <FieldError v-if="r.error" data-testid="restore-row-error">{{
                  r.error
                }}</FieldError>
              </li>
            </ul>
          </CollapsibleContent>
        </Collapsible>

        <Alert v-if="callError" variant="error" data-testid="restore-call-error">
          <AlertDescription>{{ callError }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          :pending="dismissing"
          data-testid="restore-dismiss"
          @click="emit('dismiss')"
          >{{
            t('workbench.restore.dismiss.label')
          }}</Button
        >
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="pending"
          :disabled="chosen.length === 0"
          data-testid="restore-submit"
          @click="emit('confirm', chosen)"
          >{{
            t('workbench.restore.submit.label', { count: chosen.length })
          }}</Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
