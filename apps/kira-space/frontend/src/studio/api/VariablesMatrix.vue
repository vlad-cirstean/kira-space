<script setup lang="ts">
import type { ApiVariableEdit } from '@shared/domain/apiConfig';
import type { ApiVariable } from '@shared/domain/variables';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Input } from '@theme/components/ui/input';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useFocus } from '@vueuse/core';
import { copyOrReportError, readClipboardText } from '@workbench/behaviour/clipboard';
import MenuIconButton from '@workbench/components/MenuIconButton.vue';
import { shortcutFor } from '@workbench/shortcuts/keys';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { runMenuShortcut, useContextMenuStore } from '@workbench/state/contextMenu';
import { type ComponentPublicInstance, computed, nextTick, ref, useTemplateRef, watch } from 'vue';
import EnvironmentColumnMenu from './EnvironmentColumnMenu.vue';
import InlineValueInput from './InlineValueInput.vue';
import { variableCellMenu, variableRowMenu } from './menu';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import type { MatrixModel, MatrixOwner } from './state/matrixModel';
import { useVariableSetStore, useVariablesStore } from './state/variables';
import { cellKey, useVariablesMatrixStore } from './state/variablesMatrix';
import VariableCell from './VariableCell.vue';
import VariableHistoryMenu from './VariableHistoryMenu.vue';
import { defaultAgentHosts } from './variablesForm';

// The grid: a row per name, a column per file. The collection's default column, then each
// environment in saved order; narrow mode shows one. Every commit is an EditVariables call; the file
// is the draft.
const props = defineProps<{ model: MatrixModel; narrow: boolean }>();

const matrix = useVariablesMatrixStore();
const variablesStore = useVariablesStore();
const variableSetStore = useVariableSetStore();
const dialogStore = useApiVariablesDialogStore();
const confirmDialogStore = useConfirmDialogStore();
const contextMenuStore = useContextMenuStore();

const columns = computed<MatrixOwner[]>(() =>
  props.narrow
    ? props.model.shownOwner.value
      ? [props.model.shownOwner.value]
      : []
    : props.model.owners.value,
);
const activeId = computed(() => variablesStore.activeEnvironmentId);

// Where a new row's value lands: the active environment's column when shown, else the first column.
const target = computed<MatrixOwner | null>(
  () => columns.value.find((c) => c.isActive) ?? columns.value[0] ?? null,
);

function edit(owner: MatrixOwner, name: string, op: ApiVariableEdit['op']): ApiVariableEdit {
  return { scope: owner.scope, ownerId: owner.id, op, name };
}

function ownerById(id: string): MatrixOwner | undefined {
  return props.model.owners.value.find((o) => o.id === id);
}

// ---- cells ----

const onSave = (owner: MatrixOwner, name: string, value: string) =>
  matrix.edit([{ ...edit(owner, name, 'set'), value }]);
const onSaveSecret = (owner: MatrixOwner, name: string, secretValue: string) =>
  matrix.edit([{ ...edit(owner, name, 'set'), secretValue }]);
const onClear = (owner: MatrixOwner, name: string) => matrix.edit([edit(owner, name, 'unset')]);

async function onReveal(owner: MatrixOwner, name: string): Promise<void> {
  const v = props.model.variableOf(owner.id, name);
  if (v) await variableSetStore.revealVariable(v.id, variableSetStore.setVariableSetError);
}

async function pasteInto(owner: MatrixOwner, name: string): Promise<void> {
  let text: string;
  try {
    text = await readClipboardText();
  } catch (err) {
    matrix.fail(err);
    return;
  }
  if (text === '') return;
  const secret =
    props.model.variableOf(owner.id, name)?.isSecret ?? props.model.lockOf(name) === 'all';
  await (secret ? onSaveSecret(owner, name, text) : onSave(owner, name, text));
}

const historyAnchor = ref<HTMLElement | null>(null);
async function openHistory(owner: MatrixOwner, name: string): Promise<void> {
  const v = props.model.variableOf(owner.id, name);
  if (!v) return;
  historyAnchor.value = document.querySelector<HTMLElement>(
    `[data-testid="matrix-name-${CSS.escape(name)}"]`,
  );
  await variableSetStore.openHistoryMenu(v.id);
}

function cellMenu(owner: MatrixOwner, name: string) {
  const v = props.model.variableOf(owner.id, name);
  return variableCellMenu(
    {
      edit: () => matrix.startEditing(cellKey(owner.id, name)),
      copy: () => void copyOrReportError(v?.value ?? '', matrix.fail),
      paste: () => void pasteInto(owner, name),
      clear: () => void onClear(owner, name),
      history: () => void openHistory(owner, name),
    },
    { editable: !owner.broken, canCopy: !!v && !v.isSecret, canClear: !!v, canHistory: !!v },
  );
}

// ---- rows ----

async function toggleLock(name: string): Promise<void> {
  const holders = props.model.holders(name);
  if (props.model.lockOf(name) !== 'all') {
    await matrix.edit(holders.map((o) => edit(o, name, 'lock')));
    return;
  }
  await matrix.unlock(
    holders.map((o) => edit(o, name, 'unlock')),
    () =>
      confirmDialogStore.confirm({
        title: t('studio.api.variables.unlock.confirm.title'),
        message: t('studio.api.variables.unlock.confirm.body', { name }),
        confirmLabel: t('studio.api.variables.row.unlock.label'),
        tone: 'danger',
      }),
  );
}

async function commitRowEdit(name: string, value: string): Promise<void> {
  const mode = matrix.rowEdit?.mode;
  matrix.rowEdit = null;
  const holders = props.model.holders(name);
  if (mode === 'describe') {
    await matrix.edit(holders.map((o) => ({ ...edit(o, name, 'describe'), description: value })));
    return;
  }
  const done = await matrix.edit(
    holders.map((o) => ({ ...edit(o, name, 'rename'), newName: value })),
  );
  if (done) matrix.renamed = { from: name, to: value };
}

async function removeRow(name: string): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('studio.api.variables.delete.confirm.title'),
    message: t('studio.api.variables.delete.confirm.body', { name }),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (ok) await matrix.edit(props.model.holders(name).map((o) => edit(o, name, 'unset')));
}

async function share(envId: string, name: string, on: boolean): Promise<void> {
  const owner = ownerById(envId);
  const v = props.model.variableOf(envId, name);
  if (!owner || !v) return;
  const rows = props.model.allNames.value
    .map((n) => props.model.variableOf(envId, n))
    .filter((x): x is ApiVariable => !!x);
  const hosts =
    on && v.isSecret ? defaultAgentHosts(rows.map((r) => ({ name: r.name, value: r.value }))) : [];
  if (on && v.isSecret && hosts.length === 0) {
    // A secret may only go to hosts the user names: the Agent access dialog asks for them.
    dialogStore.open(owner.file, 'agent');
    return;
  }
  await matrix.edit([{ ...edit(owner, name, 'share'), agent: on, agentHosts: hosts }]);
}

function rowMenu(name: string) {
  const holders = props.model.holders(name);
  const history = (props.model.holders(name).find((o) => o.isActive) ?? holders[0]) as
    | MatrixOwner
    | undefined;
  return variableRowMenu(
    {
      rename: () => (matrix.rowEdit = { name, mode: 'rename' }),
      describe: () => (matrix.rowEdit = { name, mode: 'describe' }),
      copyReference: () => void copyOrReportError(`{{${name}}}`, matrix.fail),
      share: (envId, on) => void share(envId, name, on),
      history: () => history && void openHistory(history, name),
      remove: () => void removeRow(name),
    },
    {
      editable: holders.every((o) => !o.broken),
      canHistory: !!history,
      environments: holders
        .filter((o) => o.scope === 'environment')
        .map((o) => ({ id: o.id, name: o.name, shared: props.model.isShared(o.id, name) })),
    },
  );
}

// ---- the new-row inputs ----

const newName = ref('');
const newValue = ref('');
const newNameInput = useTemplateRef<ComponentPublicInstance>('newNameInput');
const { focused: newNameFocused } = useFocus(newNameInput);
watch(
  () => matrix.newRowFocus,
  () => {
    newNameFocused.value = true;
  },
);

async function addRow(): Promise<void> {
  const name = newName.value.trim();
  const owner = target.value;
  if (name === '' || !owner) return;
  const done = await matrix.edit([{ ...edit(owner, name, 'set'), value: newValue.value }]);
  if (!done) return;
  newName.value = '';
  newValue.value = '';
  await nextTick();
  newNameFocused.value = true;
}

function clearNewRow(): void {
  newName.value = '';
  newValue.value = '';
}

// ---- keyboard ----

function runShortcut(e: KeyboardEvent): void {
  const id = shortcutFor(e, [
    'grid.edit',
    'grid.copy',
    'grid.paste',
    'grid.deleteRows',
    'grid.rename',
  ]);
  if (id === null) return;
  e.preventDefault();
  const el = e.currentTarget as HTMLElement;
  const name = el.dataset.name ?? '';
  const owner = ownerById(el.dataset.owner ?? '');
  if (id === 'grid.rename' || id === 'grid.deleteRows') {
    runMenuShortcut(rowMenu(name), id);
  } else if (owner) {
    runMenuShortcut(cellMenu(owner, name), id);
  } else if (id === 'grid.edit') {
    runMenuShortcut(rowMenu(name), 'grid.rename');
  }
}

const ARROW: Record<string, [number, number]> = {
  ArrowUp: [-1, 0],
  ArrowDown: [1, 0],
  ArrowLeft: [0, -1],
  ArrowRight: [0, 1],
};

// Only the cell itself handles keys: a button or input inside it keeps its own.
function onCellKeydown(e: KeyboardEvent): void {
  if (e.target !== e.currentTarget) return;
  const move = ARROW[e.key];
  if (move) {
    const [r, c] = (e.currentTarget as HTMLElement).dataset.pos?.split(':').map(Number) ?? [];
    const next = document.querySelector<HTMLElement>(
      `[data-pos="${(r ?? 0) + move[0]}:${(c ?? 0) + move[1]}"]`,
    );
    if (next) {
      e.preventDefault();
      next.focus();
    }
    return;
  }
  runShortcut(e);
}
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto" data-testid="variables-matrix-scroll">
    <RadioGroup
      class="contents"
      :aria-label="t('studio.api.environments.activeGroup.aria')"
      :model-value="activeId"
      @update:model-value="(id) => variablesStore.setActiveEnvironment(String(id))"
    >
      <Table columns class="min-w-max" :aria-label="t('studio.api.variables.grid.aria')">
        <TableHeader>
          <TableRow>
            <TableHead class="w-56">{{ t('common.field.name.label') }}</TableHead>
            <TableHead
              v-for="col in columns"
              :key="col.id"
              class="min-w-44"
              :data-testid="`matrix-col-${col.id}`"
              :data-broken="col.broken || undefined"
            >
              <div class="flex min-w-0 items-center gap-1">
                <template v-if="col.scope === 'environment'">
                  <TextTooltip
                    :text="
                      col.isActive
                        ? t('studio.api.environments.active.tooltip')
                        : t('studio.api.variables.column.makeActive.tooltip')
                    "
                  >
                    <RadioGroupItem
                      :value="col.id"
                      :aria-label="t('studio.api.environments.active.aria', { name: col.name })"
                      :data-testid="`matrix-activate-${col.id}`"
                    />
                  </TextTooltip>
                  <span :class="colorMarkClass('dot', col.color)" />
                  <span
                    class="min-w-0 truncate text-kira-md text-fg"
                    data-testid="matrix-col-name"
                    >{{
                      col.name
                    }}</span
                  >
                  <Badge v-if="col.isActive" class="ml-1">{{
                    t('studio.api.environments.active.tooltip')
                  }}</Badge>
                </template>
                <TextTooltip v-else :text="t('studio.api.variables.column.default.tooltip')"
                  ><span class="flex min-w-0 items-center gap-1 text-kira-md text-muted-foreground"
                    ><CodiconIcon name="library" :size="13" />
                    <span class="truncate" data-testid="matrix-col-name">{{ col.name }}</span></span
                  ></TextTooltip
                >
                <TextTooltip
                  v-if="col.broken"
                  :text="t('studio.api.variables.column.broken.tooltip')"
                  ><CodiconIcon :name="ICON.warning" :size="12" class="text-warn" /></TextTooltip
                >
                <span class="ml-auto"><EnvironmentColumnMenu :owner="col" /></span>
              </div>
            </TableHead>
            <TableHead class="w-10">
              <TooltipIconButton
                v-if="!narrow"
                :icon="ICON.add"
                :label="t('studio.api.environments.new.label')"
                data-testid="matrix-add-env-col"
                @click="matrix.addingEnvironment = true"
              />
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="(name, r) in model.names.value"
            :key="name"
            :data-testid="`matrix-row-${name}`"
            :data-lock="model.lockOf(name)"
          >
            <TableCell
              tabindex="0"
              class="group/name focus-visible:focus-ring"
              :data-testid="`matrix-name-${name}`"
              :data-name="name"
              :data-pos="`${r}:0`"
              @keydown="onCellKeydown"
            >
              <div class="flex min-w-0 items-center gap-1">
                <InlineValueInput
                  v-if="matrix.rowEdit?.name === name"
                  :value="matrix.rowEdit.mode === 'describe' ? model.descriptionOf(name) : name"
                  :allow-empty="matrix.rowEdit.mode === 'describe'"
                  :label="
                    matrix.rowEdit.mode === 'describe'
                      ? t('studio.api.variables.row.description.aria', { name })
                      : t('studio.api.variables.row.name.aria', { name })
                  "
                  data-testid="matrix-row-input"
                  @commit="(text) => commitRowEdit(name, text)"
                  @cancel="matrix.rowEdit = null"
                />
                <template v-else>
                  <span class="min-w-0 truncate font-data text-kira-md text-fg">{{ name }}</span>
                  <TextTooltip v-if="model.descriptionOf(name)" :text="model.descriptionOf(name)"
                    ><CodiconIcon
                      name="note"
                      :size="12"
                      class="shrink-0 text-subtle"
                    /></TextTooltip
                  >
                  <TooltipIconButton
                    class="ml-auto shrink-0"
                    :class="
                      model.lockOf(name) === 'none'
                        ? 'opacity-0 group-hover/name:opacity-100 focus-visible:opacity-100'
                        : 'text-fg'
                    "
                    :icon="model.lockOf(name) === 'none' ? 'unlock' : 'lock'"
                    :label="
                      model.lockOf(name) === 'none'
                        ? t('studio.api.variables.row.lock.label')
                        : model.lockOf(name) === 'all'
                          ? t('studio.api.variables.row.unlock.label')
                          : t('studio.api.variables.row.lockMixed.label')
                    "
                    :pressed="model.lockOf(name) === 'all'"
                    :data-testid="`matrix-secret-${name}`"
                    @click="toggleLock(name)"
                  />
                </template>
              </div>
            </TableCell>
            <VariableCell
              v-for="(col, c) in columns"
              :key="col.id"
              :owner-id="col.id"
              :owner-name="col.name"
              :name="name"
              :variable="model.variableOf(col.id, name)"
              :inherited="
                col.scope === 'environment' && model.defaultOwner.value
                  ? model.variableOf(model.defaultOwner.value.id, name)
                  : undefined
              "
              :is-default="col.scope === 'collection'"
              :collection-name="model.collection.value?.name ?? ''"
              :row-secret="model.lockOf(name) === 'all'"
              :readonly="col.broken"
              :shared="col.scope === 'environment' && model.isShared(col.id, name)"
              :overridden="col.scope === 'collection' && model.overriddenEverywhere(name)"
              :revealed="variableSetStore.revealedValues[model.variableOf(col.id, name)?.id ?? '']"
              :data-name="name"
              :data-owner="col.id"
              :data-pos="`${r}:${c + 1}`"
              @keydown="onCellKeydown"
              @contextmenu.prevent="contextMenuStore.openContextMenu($event, cellMenu(col, name))"
              @save="(v) => onSave(col, name, v)"
              @save-secret="(v) => onSaveSecret(col, name, v)"
              @clear="onClear(col, name)"
              @reveal="onReveal(col, name)"
            />
            <TableCell class="text-center">
              <MenuIconButton
                :icon="ICON.moreActions"
                :label="t('studio.api.variables.actions.aria', { name })"
                :items="() => rowMenu(name)"
                :data-testid="`matrix-row-menu-${name}`"
              />
            </TableCell>
          </TableRow>
          <TableRow class="hover:bg-transparent">
            <TableCell>
              <Input
                ref="newNameInput"
                v-model="newName"
                size="kira"
                class="font-data"
                :placeholder="t('common.action.addVariable.label')"
                :aria-label="t('common.action.addVariable.label')"
                :disabled="!target"
                data-testid="matrix-new-name"
                @keydown.enter.prevent="addRow"
                @keydown.esc.stop.prevent="clearNewRow"
              />
            </TableCell>
            <TableCell v-for="col in columns" :key="col.id">
              <Input
                v-if="target && col.id === target.id && newName !== ''"
                v-model="newValue"
                size="kira"
                class="font-data"
                :placeholder="t('studio.api.variables.new.value.placeholder', { name: col.name })"
                :aria-label="t('studio.api.variables.new.value.aria', { name: col.name })"
                data-testid="matrix-new-value"
                @keydown.enter.prevent="addRow"
                @keydown.esc.stop.prevent="clearNewRow"
              />
            </TableCell>
            <TableCell />
          </TableRow>
        </TableBody>
      </Table>
    </RadioGroup>

    <Popover
      :open="variableSetStore.open"
      @update:open="(open) => !open && variableSetStore.closeHistoryMenu()"
    >
      <PopoverAnchor :reference="historyAnchor ?? undefined" />
      <VariableHistoryMenu v-if="variableSetStore.variableId" />
    </Popover>
  </div>
</template>
