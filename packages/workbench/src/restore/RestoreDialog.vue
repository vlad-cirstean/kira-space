<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
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
import { ICON } from '@theme/icons';
import { computed, ref } from 'vue';
import RestoreRowItem from './RestoreRowItem.vue';
import type { RestoreConfirm, RestoreRow } from './types';

// Presentational: the adapters (Docker, Runs) own the data, the calls and the placement.
const props = defineProps<{
  title: string;
  description: string;
  rows: RestoreRow[];
  pending: boolean;
  dismissing?: boolean;
  /** The footer Dismiss shows only with a label: a dialog whose X already dismisses passes none. */
  dismissLabel?: string;
  callError?: string;
}>();
/** Ids the person unchecked. A ready row counts as selected unless listed, so rows that arrive later start checked. */
const excluded = defineModel<string[]>('excluded', { default: () => [] });
/** What the person set on rows with an option; a row not listed has its option's default. */
const optionChoice = defineModel<Record<string, boolean>>('options', { default: () => ({}) });
const emit = defineEmits<{ confirm: [choice: RestoreConfirm]; dismiss: []; close: [] }>();

const listed = computed(() => props.rows.filter((r) => r.state !== 'unavailable'));
const unavailable = computed(() => props.rows.filter((r) => r.state === 'unavailable'));
const ready = computed(() => props.rows.filter((r) => r.state === 'ready'));
const chosen = computed(() =>
  ready.value.filter((r) => !excluded.value.includes(r.id)).map((r) => r.id),
);
const showUnavailable = ref(false);

// Consecutive rows of one group share a heading.
const sections = computed(() => {
  const out: { group: string; rows: RestoreRow[] }[] = [];
  for (const r of listed.value) {
    const group = r.group ?? '';
    const last = out[out.length - 1];
    if (last && last.group === group) last.rows.push(r);
    else out.push({ group, rows: [r] });
  }
  return out;
});

function optionOn(r: RestoreRow): boolean {
  if (!r.option) return false;
  if (r.option.unsupportedReason) return false;
  if (r.option.required) return true;
  return optionChoice.value[r.id] ?? r.option.default ?? false;
}

function toggleOption(id: string, on: boolean): void {
  optionChoice.value = { ...optionChoice.value, [id]: on };
}

const confirmed = computed<RestoreConfirm>(() => ({
  ids: chosen.value,
  options: ready.value.filter((r) => chosen.value.includes(r.id) && optionOn(r)).map((r) => r.id),
}));

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
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription>{{ description }}</DialogDescription>
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
              ><CodiconIcon name="check-all" :size="13" />
              {{ t('common.action.selectAll.label') }}</Button
            >
            <Button
              variant="toolbar"
              size="kira"
              :disabled="chosen.length === 0"
              data-testid="restore-select-none"
              @click="excluded = ready.map((r) => r.id)"
              ><CodiconIcon :name="ICON.clearAll" :size="13" />
              {{ t('common.action.selectNone.label') }}</Button
            >
          </div>
        </div>

        <section
          v-for="sec in sections"
          :key="sec.group"
          :data-testid="sec.group ? 'restore-group' : undefined"
          :data-group="sec.group || undefined"
        >
          <SectionHeading v-if="sec.group" :label="sec.group" />
          <ul class="m-0 flex list-none flex-col rounded-kira">
            <li
              v-for="r in sec.rows"
              :key="r.id"
              data-testid="restore-row"
              :data-id="r.id"
              :data-state="r.state"
              :data-status="r.status"
            >
              <RestoreRowItem
                :row="r"
                :checked="isChecked(r)"
                :option-on="isChecked(r) && optionOn(r)"
                @toggle="(on) => toggle(r.id, on)"
                @toggle-option="(on) => toggleOption(r.id, on)"
              />
            </li>
          </ul>
        </section>

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
          v-if="dismissLabel"
          variant="dialog"
          size="kira-lg"
          :pending="dismissing"
          data-testid="restore-dismiss"
          @click="emit('dismiss')"
          >{{
            dismissLabel
          }}</Button
        >
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="pending"
          :disabled="chosen.length === 0"
          data-testid="restore-submit"
          @click="emit('confirm', confirmed)"
          >{{
            t('workbench.restore.submit.label', { count: chosen.length })
          }}</Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
