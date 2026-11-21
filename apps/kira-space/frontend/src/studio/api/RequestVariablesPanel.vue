<script setup lang="ts">
import { isDynamicName, isFakeName } from '@kira/api-core';
import type { ApiVariable } from '@shared/domain/variables';
import CodiconIcon from '@theme/CodiconIcon.vue';
import MaskedValue from '@theme/components/MaskedValue.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { PopoverContent } from '@theme/components/ui/popover';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import MenuIconButton from '@workbench/components/MenuIconButton.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import InlineValueInput from './InlineValueInput.vue';
import { useVariableRows } from './state/apiQueries';
import { useCollectionsStore } from './state/collections';
import { useVariablesStore } from './state/variables';
import { useVariablesMatrixStore } from './state/variablesMatrix';

// The variables this request uses, resolved, each with where its value comes from. A missing one has
// an input to add it to a file; a plain value edits in place in its own file.
const props = defineProps<{
  collectionId: string;
  environmentId: string;
  refs: readonly { kind: string; name: string }[];
  /** An incognito tab writes nothing. */
  readonly?: boolean;
}>();
const emit = defineEmits<{ close: [] }>();

const collectionsStore = useCollectionsStore();
const variablesStore = useVariablesStore();
const matrix = useVariablesMatrixStore();
const collectionRows = useVariableRows('collection', () => props.collectionId);
const environmentRows = useVariableRows('environment', () => props.environmentId);

const environment = computed(() =>
  variablesStore.environments.find((e) => e.id === props.environmentId),
);
const collectionName = computed(
  () => collectionsStore.collectionRecord(props.collectionId)?.name ?? '',
);

type RowState = 'environment' | 'collection' | 'missing';
interface Row {
  name: string;
  state: RowState;
  variable?: ApiVariable;
  /** The environment value hides a collection default of the same name. */
  overrides: boolean;
}

const rows = computed<Row[]>(() => {
  const seen = new Set<string>();
  const out: Row[] = [];
  for (const ref of props.refs) {
    if (seen.has(ref.name) || isDynamicName(ref.name) || isFakeName(ref.name)) continue;
    seen.add(ref.name);
    const env = (environmentRows.data.value ?? []).find((v) => v.name === ref.name);
    const base = (collectionRows.data.value ?? []).find((v) => v.name === ref.name);
    if (env) out.push({ name: ref.name, state: 'environment', variable: env, overrides: !!base });
    else if (base)
      out.push({ name: ref.name, state: 'collection', variable: base, overrides: false });
    else out.push({ name: ref.name, state: 'missing', overrides: false });
  }
  return out.sort((a, b) => Number(b.state === 'missing') - Number(a.state === 'missing'));
});

// ---- adding a missing variable ----

/** Where a typed value goes: an environment id, or '' for the collection default. */
const targetId = ref<string | null>(null);
const target = computed(() => {
  const id = targetId.value ?? props.environmentId;
  if (id !== '' && variablesStore.environments.some((e) => e.id === id)) {
    const env = variablesStore.environments.find((e) => e.id === id);
    return { scope: 'environment' as const, id, name: env?.name ?? '' };
  }
  return { scope: 'collection' as const, id: props.collectionId, name: collectionName.value };
});
const canTarget = computed(() => target.value.id !== '');

const targetItems = (): MenuItem[] => [
  {
    type: 'radio',
    id: 'target',
    value: target.value.scope === 'collection' ? '' : target.value.id,
    items: [
      {
        type: 'option',
        id: 'target-collection',
        value: '',
        label: t('studio.api.variables.request.targetDefault.option', {
          name: collectionName.value,
        }),
        run: () => {
          targetId.value = '';
        },
      },
      ...variablesStore.environments.map((e) => ({
        type: 'option' as const,
        id: `target-${e.id}`,
        value: e.id,
        swatch: e.color,
        label: e.name,
        run: () => {
          targetId.value = e.id;
        },
      })),
    ],
  },
];

const fix = ref<Record<string, string>>({});
async function add(name: string): Promise<void> {
  const value = fix.value[name] ?? '';
  if (value === '' || !canTarget.value) return;
  const done = await matrix.edit([
    { scope: target.value.scope, ownerId: target.value.id, op: 'set', name, value },
  ]);
  if (done) delete fix.value[name];
}

// ---- editing a value in its own file ----

const editingName = ref<string | null>(null);

function begin(row: Row): void {
  if (props.readonly || row.variable?.isSecret || !row.variable) return;
  editingName.value = row.name;
}

async function commit(row: Row, value: string): Promise<void> {
  editingName.value = null;
  await matrix.edit([
    {
      scope: row.state === 'environment' ? 'environment' : 'collection',
      ownerId: row.state === 'environment' ? props.environmentId : props.collectionId,
      op: 'set',
      name: row.name,
      value,
    },
  ]);
}

function sourceTooltip(row: Row): string {
  if (row.state === 'environment') {
    return row.overrides
      ? t('studio.api.variables.request.overrides.tooltip', { collection: collectionName.value })
      : t('studio.api.variables.request.fromEnvironment.tooltip', {
          name: environment.value?.name ?? '',
        });
  }
  return t('studio.api.variables.request.fromCollection.tooltip', { name: collectionName.value });
}

function openAll(): void {
  variablesStore.openEnvironments(props.collectionId || undefined);
  emit('close');
}
</script>

<template>
  <PopoverContent align="end" flush class="w-120" data-testid="request-variables">
    <div class="flex flex-col">
      <div class="flex items-center gap-2 border-b border-border px-3 py-2">
        <span class="text-kira-md text-fg">{{
          t('studio.api.variables.request.title.label')
        }}</span>
        <span class="ml-auto flex items-center gap-1.5 text-kira-sm text-muted-foreground">
          {{ t('studio.api.variables.request.resolvedWith.label') }}
          <span v-if="environment" :class="colorMarkClass('dot', environment.color)" />
          {{ environment?.name ?? t('studio.api.environments.none.option') }}
        </span>
      </div>
      <Alert v-if="matrix.error" variant="error" strip data-testid="request-variables-error">
        <AlertDescription>{{ matrix.error }}</AlertDescription>
      </Alert>
      <div class="flex max-h-80 flex-col overflow-auto py-1">
        <div
          v-if="rows.length === 0"
          class="px-3 py-2 text-kira-sm text-subtle"
          data-testid="request-variables-empty"
          >{{
            t('studio.api.variables.request.empty.description')
          }}</div
        >
        <div
          v-for="row in rows"
          :key="row.name"
          class="grid min-h-9 grid-cols-[9rem_1fr_auto] items-center gap-2 px-3"
          :data-testid="`request-var-${row.name}`"
          :data-state="row.state"
        >
          <span
            class="min-w-0 truncate font-data text-kira-md"
            :class="row.state === 'missing' ? 'text-warn' : 'text-fg'"
            >{{
              row.name
            }}</span
          >
          <template v-if="row.state === 'missing'">
            <Input
              v-model="fix[row.name]"
              size="kira"
              class="font-data"
              :placeholder="t('studio.api.variables.new.value.placeholder', { name: target.name })"
              :aria-label="t('studio.api.variables.request.fix.aria', { name: row.name })"
              :disabled="readonly || !canTarget"
              data-testid="request-var-fix-input"
              @keydown.enter.prevent="add(row.name)"
            />
            <span class="flex items-center">
              <Button
                variant="toolbar"
                size="kira"
                :disabled="readonly || !canTarget || !fix[row.name]"
                data-testid="request-var-fix-add"
                @click="add(row.name)"
                ><CodiconIcon :name="ICON.add" :size="13" />
                {{ t('studio.api.variables.request.fix.label', { name: target.name }) }}</Button
              >
              <MenuIconButton
                icon="chevron-down"
                :label="t('studio.api.variables.request.target.aria')"
                :items="targetItems"
                data-testid="request-var-fix-target"
              />
            </span>
          </template>
          <template v-else>
            <MaskedValue v-if="row.variable?.isSecret" />
            <InlineValueInput
              v-else-if="editingName === row.name"
              :value="row.variable?.value ?? ''"
              :label="t('studio.api.variables.request.fix.aria', { name: row.name })"
              data-testid="request-var-input"
              @commit="(text) => commit(row, text)"
              @cancel="editingName = null"
            />
            <button
              v-else
              type="button"
              class="min-w-0 cursor-text text-left font-data text-kira-md text-muted-foreground"
              :disabled="readonly"
              :data-testid="`request-var-value-${row.name}`"
              @click="begin(row)"
            >
              <TruncatedText :text="row.variable?.value ?? ''" />
            </button>
            <TextTooltip :text="sourceTooltip(row)">
              <span class="flex items-center gap-1 text-kira-sm text-subtle">
                <span
                  v-if="row.state === 'environment'"
                  :class="colorMarkClass('dot', environment?.color)"
                />
                <CodiconIcon v-else name="library" :size="12" />
                {{ row.state === 'environment' ? environment?.name : collectionName }}
              </span>
            </TextTooltip>
          </template>
        </div>
      </div>
      <div class="flex items-center gap-2 border-t border-border px-2 py-1.5">
        <span class="px-1 text-kira-sm text-subtle">{{
          t('studio.api.variables.request.edit.hint')
        }}</span>
        <Button
          variant="toolbar"
          size="kira"
          class="ml-auto"
          shortcut="api.environments"
          data-testid="request-variables-all"
          @click="openAll"
          ><CodiconIcon :name="ICON.variables" :size="13" />
          {{ t('studio.api.variables.request.all.label') }}</Button
        >
      </div>
    </div>
  </PopoverContent>
</template>
