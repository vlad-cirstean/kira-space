<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { type MessageKey, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { errorText } from '@workbench/bridge/codedError';
import SortableList from '@workbench/components/SortableList.vue';
import { computed, ref } from 'vue';
import { useSaveStatuses } from '../../queries';
import { useBoard, useWorkflows } from '../../readQueries';
import { STATUS_COLORS, STATUS_DOT_CLASS, STATUS_SWATCH_CLASS } from '../../statusColors';
import type { StatusColor, StatusDefaults, TaskStatusDef } from '../../wire';
import AdeStatusName from './AdeStatusName.vue';

// Self-saving: every edit writes the whole set and the defaults at once, like a Plan write, so no draft
// or Save. Editing a status never touches a task: tasks keep the status id they hold.

type Slot = keyof StatusDefaults;
const SLOTS: readonly Slot[] = ['new', 'step', 'review', 'finished'];
const SLOT_LABEL: Record<Slot, MessageKey> = {
  new: 'space.ade.statuses.defaults.new.label',
  step: 'space.ade.statuses.defaults.step.label',
  review: 'space.ade.statuses.defaults.review.label',
  finished: 'space.ade.statuses.defaults.finished.label',
};
const SLOT_HINT: Record<Slot, MessageKey> = {
  new: 'space.ade.statuses.defaults.new.hint',
  step: 'space.ade.statuses.defaults.step.hint',
  review: 'space.ade.statuses.defaults.review.hint',
  finished: 'space.ade.statuses.defaults.finished.hint',
};

const board = useBoard();
const workflows = useWorkflows();
const save = useSaveStatuses();
const statuses = computed(() => board.data.value?.statuses ?? []);
const defaults = computed<StatusDefaults>(
  () => board.data.value?.statusDefaults ?? { new: '', step: '', review: '', finished: '' },
);
const ids = computed(() => statuses.value.map((s) => s.id));
const error = ref('');
const fresh = ref('');

const colorItems = STATUS_COLORS.map((c) => ({
  value: c,
  label: t(`common.colour.${c}.label` as MessageKey),
}));

async function commit(
  next: TaskStatusDef[],
  nextDefaults: StatusDefaults = defaults.value,
  reassign: Record<string, string> = {},
): Promise<TaskStatusDef[] | null> {
  error.value = '';
  try {
    return (await save.mutateAsync({ statuses: next, defaults: nextDefaults, reassign })).statuses;
  } catch (err) {
    error.value = errorText(err);
    return null;
  }
}

function edit(id: string, patch: Partial<TaskStatusDef>): Promise<TaskStatusDef[] | null> {
  return commit(statuses.value.map((s) => (s.id === id ? { ...s, ...patch } : s)));
}

async function rename(s: TaskStatusDef, name: string): Promise<void> {
  if (!(await edit(s.id, { name }))) throw new Error(error.value);
}

function setDefault(slot: Slot, id: string): void {
  void commit(statuses.value, { ...defaults.value, [slot]: id });
}

function uniqueName(): string {
  const base = t('space.ade.statuses.newName.label');
  const taken = new Set(statuses.value.map((s) => s.name.toLowerCase()));
  let name = base;
  for (let n = 2; taken.has(name.toLowerCase()); n++) name = `${base} ${n}`;
  return name;
}

async function add(): Promise<void> {
  const before = new Set(ids.value);
  const used = new Set(statuses.value.map((s) => s.color));
  const color: StatusColor = STATUS_COLORS.find((c) => !used.has(c)) ?? 'grey';
  const saved = await commit([...statuses.value, { id: '', name: uniqueName(), color }]);
  if (saved) fresh.value = saved.find((s) => !before.has(s.id))?.id ?? '';
}

function move(from: string, to: string): void {
  const next = [...statuses.value];
  const i = next.findIndex((s) => s.id === from);
  const j = next.findIndex((s) => s.id === to);
  if (i < 0 || j < 0) return;
  next.splice(j, 0, ...next.splice(i, 1));
  void commit(next);
}

// ---- delete
const removing = ref<TaskStatusDef | null>(null);
const reassignTo = ref('');
const targets = computed(() => statuses.value.filter((s) => s.id !== removing.value?.id));

const uses = computed(() => {
  const id = removing.value?.id;
  if (!id) return { tasks: 0, slots: [] as string[], steps: 0 };
  const entries = workflows.data.value?.workflows ?? [];
  return {
    tasks: (board.data.value?.tasks ?? []).filter((task) => task.statusId === id).length,
    slots: SLOTS.filter((slot) => defaults.value[slot] === id).map((slot) => t(SLOT_LABEL[slot])),
    steps: entries.reduce(
      (n, e) => n + (e.workflow?.steps.filter((st) => st.status === id).length ?? 0),
      0,
    ),
  };
});

function remove(s: TaskStatusDef): void {
  error.value = '';
  removing.value = s;
  reassignTo.value = targets.value[0]?.id ?? '';
}

async function confirmRemove(): Promise<void> {
  const s = removing.value;
  if (!s) return;
  const moved = Object.fromEntries(
    SLOTS.map((slot) => [
      slot,
      defaults.value[slot] === s.id ? reassignTo.value : defaults.value[slot],
    ]),
  ) as unknown as StatusDefaults;
  const ok = await commit(
    statuses.value.filter((o) => o.id !== s.id),
    moved,
    { [s.id]: reassignTo.value },
  );
  if (ok) removing.value = null;
}
</script>

<template>
  <div class="flex max-w-2xl flex-col gap-4" data-testid="ade-task-statuses">
    <Field>
      <FieldDescription>{{ t('space.ade.statuses.description') }}</FieldDescription>
      <div class="flex flex-col gap-1" data-testid="ade-status-list">
        <SortableList
          :ids="ids"
          draggable="[data-status-id]"
          handle='[data-testid="ade-status-handle"]'
          keyboard-handle='[data-testid="ade-status-handle"]'
          class="flex flex-col gap-1"
          @move="move"
        >
          <div
            v-for="s in statuses"
            :key="s.id"
            class="flex items-center gap-2"
            :data-status-id="s.id"
            data-testid="ade-status-row"
          >
            <button
              type="button"
              class="cursor-grab text-muted-foreground"
              :aria-label="t('mobile.backlog.handle.aria')"
              data-testid="ade-status-handle"
            >
              <CodiconIcon name="gripper" :size="13" />
            </button>
            <Popover>
              <PopoverTrigger as-child>
                <Button
                  variant="toolbar"
                  size="kira"
                  :aria-label="t('common.field.colour.label')"
                  :data-color="s.color"
                  data-testid="ade-status-color"
                >
                  <span class="size-3 rounded-full" :class="STATUS_DOT_CLASS[s.color]" />
                </Button>
              </PopoverTrigger>
              <PopoverContent align="start" class="w-auto" data-testid="ade-status-colors">
                <RadioGroup
                  :model-value="s.color"
                  :aria-label="t('common.field.colour.label')"
                  class="grid grid-cols-8 gap-1.5"
                  @update:model-value="
                    (v) => v !== s.color && edit(s.id, { color: String(v) as StatusColor })
                  "
                >
                  <RadioGroupItem
                    v-for="c in colorItems"
                    :key="c.value"
                    :value="c.value"
                    :aria-label="c.label"
                    :title="c.label"
                    :class="[
                      'size-4 border-transparent data-[state=checked]:border-fg',
                      STATUS_SWATCH_CLASS[c.value],
                    ]"
                    :data-testid="`ade-status-color-${c.value}`"
                  >
                    <span />
                  </RadioGroupItem>
                </RadioGroup>
              </PopoverContent>
            </Popover>
            <AdeStatusName
              class="min-w-0 flex-1"
              :name="s.name"
              :label="t('common.field.name.label')"
              :focus="s.id === fresh"
              :commit="(name) => rename(s, name)"
              data-testid="ade-status-name"
            />
            <TooltipIconButton
              :icon="ICON.delete"
              :label="
                statuses.length < 2
                  ? t('space.ade.statuses.lastOne.tooltip')
                  : t('space.ade.statuses.delete.tooltip')
              "
              :aria-label="t('space.ade.statuses.delete.tooltip')"
              :disabled="statuses.length < 2"
              disabled-trigger
              data-testid="ade-status-delete"
              @click="remove(s)"
            />
          </div>
        </SortableList>
      </div>
      <FieldError v-if="error && !removing" data-testid="ade-status-error">{{ error }}</FieldError>
      <div>
        <Button variant="dialog" size="kira-lg" data-testid="ade-status-add" @click="add">
          <CodiconIcon :name="ICON.add" :size="13" />
          {{ t('space.ade.statuses.add.label') }}
        </Button>
      </div>
    </Field>

    <FieldSet>
      <FieldLegend>{{ t('space.ade.statuses.defaults.title') }}</FieldLegend>
      <FieldDescription>{{ t('space.ade.statuses.defaults.description') }}</FieldDescription>
      <div class="@container flex flex-col gap-2" data-testid="ade-status-defaults">
        <Field v-for="slot in SLOTS" :key="slot" orientation="responsive">
          <FieldLabel :for="`ade-status-default-${slot}`">{{ t(SLOT_LABEL[slot]) }}</FieldLabel>
          <NativeSelect
            :id="`ade-status-default-${slot}`"
            variant="bordered"
            :model-value="defaults[slot]"
            :data-testid="`ade-status-default-${slot}`"
            @update:model-value="(v) => setDefault(slot, String(v))"
          >
            <option v-if="slot !== 'new'" value="">{{
              t('space.ade.statuses.defaults.keep.label')
            }}</option>
            <option v-for="s in statuses" :key="s.id" :value="s.id">{{ s.name }}</option>
          </NativeSelect>
          <FieldDescription>{{ t(SLOT_HINT[slot]) }}</FieldDescription>
        </Field>
      </div>
    </FieldSet>

    <Dialog :open="removing !== null" @update:open="(o) => !o && (removing = null)">
      <DialogContent size="sm" :dirty="false" data-testid="ade-status-delete-dialog">
        <DialogHeader>
          <DialogTitle>{{
            t('space.ade.statuses.deleteDialog.title', { status: removing?.name ?? '' })
          }}</DialogTitle>
          <DialogDescription>{{ t('space.ade.statuses.deleteDialog.body') }}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <ul
            class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-md"
            data-testid="ade-status-uses"
          >
            <li v-if="uses.tasks > 0">
              {{ t('space.ade.statuses.deleteDialog.tasks.body', { count: uses.tasks }) }}
            </li>
            <li v-if="uses.slots.length > 0">
              {{
                t('space.ade.statuses.deleteDialog.defaults.body', { slots: uses.slots.join(', ') })
              }}
            </li>
            <li v-if="uses.steps > 0">
              {{ t('space.ade.statuses.deleteDialog.steps.body', { count: uses.steps }) }}
            </li>
          </ul>
          <Field>
            <FieldLabel for="ade-status-reassign">{{
              t('space.ade.statuses.reassign.label')
            }}</FieldLabel>
            <NativeSelect
              id="ade-status-reassign"
              v-model="reassignTo"
              variant="bordered"
              data-testid="ade-status-reassign"
            >
              <option v-for="o in targets" :key="o.id" :value="o.id">{{ o.name }}</option>
            </NativeSelect>
            <FieldError v-if="error" data-testid="ade-status-delete-error">{{ error }}</FieldError>
          </Field>
        </DialogBody>
        <DialogFooter>
          <Button variant="dialog" size="kira-lg" @click="removing = null">{{
            t('common.action.cancel.label')
          }}</Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="ade-status-delete-confirm"
            @click="confirmRemove"
            >{{
              t('common.action.delete.label')
            }}</Button
          >
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
