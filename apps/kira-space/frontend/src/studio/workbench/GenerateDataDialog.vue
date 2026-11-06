<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import Spinner from '@theme/components/Spinner.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, onMounted, ref } from 'vue';
import { control } from '../bridge/control';
import { data } from '../bridge/data';
import MonacoHost from '../editor/MonacoHost.vue';
import { useConnectionsStore } from '../state/connections';
import { useFakeDataStore } from '../state/fakeData';
import { useTabsStore } from '../state/tabs';
import {
  BATCH_SIZE,
  GenerationError,
  previewFirstRows,
  runGeneration,
} from '../views/grid/fakeData/generate';
import { planWarnings, RECIPE_CATALOG, recipeFor } from '../views/grid/fakeData/recipes';
import type { ColumnPlan, GeneratorId, Recipe } from '../views/grid/fakeData/types';
import { getPage } from '../views/grid/page';
import { useGridViewStore } from '../views/grid/state';
import { sqlDialectFor } from '../views/shared/sqlIdent';

// Driven by state/fakeData.ts's own open/close state, mounted in App.vue beside
// UploadObjectDialog.vue — the established shape for a feature dialog reachable from the toolbar
// and the command palette alike. App.vue mounts this fresh (v-if) on every open, so onMounted below
// is the one and only place a run's fields get their starting values.

const fakeDataStore = useFakeDataStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const gridViewStore = useGridViewStore();
const tabId = computed(() => fakeDataStore.tabId);
const tab = computed(() => (tabId.value ? tabsStore.findDataTab(tabId.value) : null));
const connRecord = computed(() => connectionsStore.connectionRecord(tab.value?.connectionId));
const caps = computed(() =>
  tab.value?.connectionId ? (connectionsStore.states[tab.value.connectionId]?.caps ?? null) : null,
);
const meta = computed(() =>
  tabId.value ? (gridViewStore.runtime[tabId.value]?.meta ?? null) : null,
);

const rowCount = ref(100);
const seed = ref(0);
const plans = ref<ColumnPlan[]>([]);
const initialPlans = ref('[]');
const dirty = computed(
  () => rowCount.value !== 100 || JSON.stringify(plans.value) !== initialPlans.value,
);

const previewOpen = ref(false);
const previewStatements = ref<string[]>([]);
const committedRows = ref(0);
const runError = ref<string | null>(null);
const currentOpId = ref<string | null>(null);
let abortController: AbortController | null = null;
let generationFailure: unknown = null;

const {
  run: runPreview,
  pending: previewLoading,
  error: previewError,
} = useAsyncAction(async (connectionId: string, path: string) => {
  // D10: the same seed as a real run, so this is literally the first rows that run would write.
  const ops = await previewFirstRows(
    plans.value,
    seed.value,
    Math.min(5, rowCount.value),
    sqlDialect.value,
  );
  previewStatements.value = ops.length
    ? (await data.preview({ connectionId, path, ops })).statements
    : [];
});

// Failures are kept for onGenerate to word (GenerationError carries committed-row counts), so the
// action itself resolves either way.
const { run: runGenerate, pending: running } = useAsyncAction(
  async (t: { connectionId: string; path: string; id: string }) => {
    generationFailure = null;
    abortController = new AbortController();
    try {
      await runGeneration({
        connectionId: t.connectionId,
        path: t.path,
        tabId: t.id,
        plans: plans.value,
        total: rowCount.value,
        seed: seed.value,
        dialect: sqlDialect.value,
        onBatchStart: (opId) => {
          currentOpId.value = opId;
        },
        onProgress: (rows) => {
          committedRows.value = rows;
        },
        signal: abortController.signal,
      });
    } catch (err) {
      generationFailure = err;
    }
  },
);

// D9: recomputed on every plan change, not baked into recipeFor's own one-time proposal.
const warnings = computed(() => planWarnings(plans.value, meta.value));

// With no plans at all (the tab's page hadn't loaded yet when the dialog
// opened, so `page?.columns ?? []` came back empty) or every plan set to Skip, a run has no column
// left to write — generateBatch would emit `{kind:'insert', values:{}}` per row, which sqlmutate.go
// turns into `INSERT INTO t () VALUES ()`: silently committed all-defaults rows on MySQL/MariaDB,
// a confusing syntax error on Postgres/SQLite. Neither Generate nor Preview should ever run then.
const noColumnsLoaded = computed(() => plans.value.length === 0);
const allColumnsSkipped = computed(
  () => plans.value.length > 0 && plans.value.every((p) => p.recipe.kind === 'skip'),
);
const noUsableColumns = computed(() => noColumnsLoaded.value || allColumnsSkipped.value);

onMounted(() => {
  const id = tabId.value;
  const page = id ? getPage(id) : undefined;
  plans.value = (page?.columns ?? []).map((c) => recipeFor(c, meta.value));
  initialPlans.value = JSON.stringify(plans.value);
  rowCount.value = 100;
  // D8: a fresh random seed per open, shown and editable — pinning it is what makes a run
  // reproducible, and it is also what makes the Preview panel show the run's real first rows.
  seed.value = Math.floor(Math.random() * 1_000_000_000);
});

function onClose(): void {
  if (running.value) return; // Stop first — a run in flight owns the dialog until it stops.
  fakeDataStore.closeGenerateDataDialog();
}

const sqlDialect = computed(() => sqlDialectFor(connRecord.value?.kind));
const previewDoc = computed(
  () => previewStatements.value.join(';\n\n') + (previewStatements.value.length ? ';' : ''),
);

async function onTogglePreview(): Promise<void> {
  if (noUsableColumns.value) return;
  previewOpen.value = !previewOpen.value;
  if (!previewOpen.value) return;
  const active = tab.value;
  if (!active?.connectionId) return;
  await runPreview(active.connectionId, active.path);
}

async function onGenerate(): Promise<void> {
  const active = tab.value;
  if (
    !active?.connectionId ||
    running.value ||
    noUsableColumns.value ||
    gridViewStore.isStale(active.id)
  )
    return;
  runError.value = null;
  committedRows.value = 0;
  await runGenerate({ connectionId: active.connectionId, path: active.path, id: active.id });
  currentOpId.value = null;
  const err = generationFailure;
  if (err === null) {
    fakeDataStore.closeGenerateDataDialog();
    await gridViewStore.reloadAfterMutation(active.id);
    return;
  }
  if (err instanceof GenerationError) {
    committedRows.value = err.committedRows;
    // D7: the server's own message, how many rows already committed, and — read from
    // caps.transactions, never assumed — whether the failing batch itself rolled back.
    runError.value =
      err.code === 'E_CANCELLED'
        ? t('studio.generateData.stopped.error', { count: err.committedRows })
        : t(
            caps.value?.transactions
              ? 'studio.generateData.rolledBack.error'
              : 'studio.generateData.noTransactions.error',
            { message: err.message, count: err.committedRows },
          );
  } else {
    runError.value = errorText(err);
  }
  if (committedRows.value > 0) await gridViewStore.reloadAfterMutation(active.id);
}

function onStop(): void {
  abortController?.abort();
  if (currentOpId.value) fireAndForget(control.opsCancel(currentOpId.value), 'cancelGenerateData');
}

type RecipeSelectValue = 'skip' | 'null' | 'constant' | 'sequence' | GeneratorId;

function recipeSelectValue(recipe: Recipe): RecipeSelectValue {
  return recipe.kind === 'faker' ? recipe.generatorId : recipe.kind;
}

function optionsFor(plan: ColumnPlan) {
  return RECIPE_CATALOG.filter((o) => o.typeClasses.includes(plan.column.typeClass));
}

function patchPlan(index: number, recipe: Recipe): void {
  plans.value = plans.value.map((p, i) => (i === index ? { ...p, recipe } : p));
}

function onRecipeChange(index: number, value: string): void {
  if (value === 'skip') patchPlan(index, { kind: 'skip' });
  else if (value === 'null') patchPlan(index, { kind: 'null' });
  else if (value === 'constant') patchPlan(index, { kind: 'constant', value: '' });
  else if (value === 'sequence') patchPlan(index, { kind: 'sequence', start: 1 });
  else patchPlan(index, { kind: 'faker', generatorId: value as GeneratorId });
}

function onConstantChange(index: number, value: string): void {
  patchPlan(index, { kind: 'constant', value });
}

function onSequenceStartChange(index: number, start: number): void {
  patchPlan(index, { kind: 'sequence', start });
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && onClose()">
    <DialogContent size="xl" :dirty="dirty" data-testid="generate-data-dialog">
      <DialogHeader close-testid="generate-data-close">
        <DialogTitle>{{ t('studio.shortcuts.palette.generateData.label') }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <div class="flex items-center gap-1.5">
          <Label class="p-0 text-muted-foreground">{{ t('studio.keyValue.rows.aria') }}</Label>
          <NumberStepperInput
            :model-value="String(rowCount)"
            group-class="h-control-lg"
            data-testid="generate-data-row-count"
            :disabled="running"
            @update:model-value="
              (v: string | number) => (rowCount = Math.max(1, Math.trunc(Number(v)) || 1))
            "
          />
          <Label class="p-0 text-muted-foreground">{{ t('studio.generateData.seed.label') }}</Label>
          <NumberStepperInput
            :model-value="String(seed)"
            group-class="h-control-lg"
            data-testid="generate-data-seed"
            :disabled="running"
            @update:model-value="(v: string | number) => (seed = Math.trunc(Number(v)) || 0)"
          />
          <span
            v-if="rowCount > BATCH_SIZE"
            :class="rowMeta"
            data-testid="generate-data-batch-note"
          >
            {{
              t('studio.generateData.batches.status', {
                count: Math.ceil(rowCount / BATCH_SIZE),
                size: BATCH_SIZE,
              })
            }}
          </span>
        </div>

        <div class="max-h-64 overflow-y-auto">
          <Table>
            <TableHeader>
              <TableRow class="hover:bg-transparent">
                <TableHead>{{ t('studio.project.connectionDialog.column.placeholder') }}</TableHead>
                <TableHead>{{ t('common.field.type.label') }}</TableHead>
                <TableHead>{{ t('studio.generateData.recipe.label') }}</TableHead>
                <TableHead>{{ t('common.field.value.label') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow
                v-for="(plan, index) in plans"
                :key="plan.column.name"
                data-testid="generate-data-column-row"
                :data-column="plan.column.name"
              >
                <TableCell class="col-name">{{ plan.column.name }}</TableCell>
                <TableCell class="max-w-32 text-muted-foreground">
                  <TruncatedText :text="plan.column.dataType" />
                </TableCell>
                <TableCell>
                  <NativeSelect
                    variant="bordered"
                    :data-testid="`generate-data-recipe-${plan.column.name}`"
                    :model-value="recipeSelectValue(plan.recipe)"
                    :disabled="running"
                    @update:model-value="(v) => onRecipeChange(index, String(v))"
                  >
                    <option value="skip">{{ t('common.skip.label') }}</option>
                    <option value="null">{{ t('studio.common.nullValue.label') }}</option>
                    <option value="constant">{{
                      t('studio.generateData.recipe.constant.label')
                    }}</option>
                    <option v-if="plan.column.typeClass === 'number'" value="sequence">{{
                      t('studio.common.nodeKind.sequence.label')
                    }}</option>
                    <option v-for="opt in optionsFor(plan)" :key="opt.id" :value="opt.id">
                      {{ t(opt.labelKey) }}
                    </option>
                  </NativeSelect>
                </TableCell>
                <TableCell>
                  <Input
                    v-if="plan.recipe.kind === 'constant'"
                    :model-value="plan.recipe.value"
                    class="w-full font-data"
                    :data-testid="`generate-data-constant-${plan.column.name}`"
                    :disabled="running"
                    @update:model-value="(v) => onConstantChange(index, String(v))"
                  />
                  <NumberStepperInput
                    v-else-if="plan.recipe.kind === 'sequence'"
                    :model-value="String(plan.recipe.start)"
                    group-class="h-control-lg"
                    :data-testid="`generate-data-sequence-start-${plan.column.name}`"
                    :disabled="running"
                    @update:model-value="
                      (v: string | number) =>
                        onSequenceStartChange(index, Math.trunc(Number(v)) || 0)
                    "
                  />
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <Alert v-if="noColumnsLoaded" variant="warn" data-testid="generate-data-no-columns">
          <AlertDescription>
            {{ t('studio.generateData.noColumns.body') }}
          </AlertDescription>
        </Alert>
        <Alert v-else-if="allColumnsSkipped" variant="warn" data-testid="generate-data-no-columns">
          <AlertDescription>
            {{ t('studio.generateData.allSkipped.body') }}
          </AlertDescription>
        </Alert>

        <Alert v-if="warnings.length" variant="warn" data-testid="generate-data-warnings">
          <AlertDescription>
            <ul class="m-0 pl-2">
              <li v-for="w in warnings" :key="w">{{ w }}</li>
            </ul>
          </AlertDescription>
        </Alert>

        <div class="preview-section">
          <Button
            type="button"
            variant="link"
            size="inline"
            data-testid="generate-data-preview-toggle"
            :disabled="noUsableColumns"
            @click="onTogglePreview"
          >
            {{
              previewOpen
                ? t('studio.generateData.hidePreview.label')
                : t('studio.grid.previewCommand.title')
            }}
          </Button>
          <div v-if="previewOpen" class="h-52 mt-1" data-testid="generate-data-preview">
            <InlineState v-if="previewLoading" kind="loading">{{
              t('common.state.loading.loading')
            }}</InlineState>
            <InlineState
              v-else-if="previewError"
              kind="error"
              data-testid="generate-data-preview-error"
              >{{
                previewError
              }}</InlineState
            >
            <MonacoHost
              v-else
              :doc="previewDoc"
              language="sql"
              :sql-dialect="sqlDialect"
              readonly
            />
          </div>
        </div>

        <Alert v-if="runError" variant="error" data-testid="generate-data-error">
          <AlertDescription>{{ runError }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <!-- P104 §3: RunState inlined -- status is always 'running' here (elapsedMs is always
             null, so the label is always the em dash RunState's own computed would give it). -->
        <Tooltip v-if="running">
          <TooltipTrigger as-child>
            <span class="inline-flex items-center text-info">
              <Spinner :size="12" label="" />
            </span>
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.generateData.progress.tooltip', { committed: committedRows, count: rowCount })
          }}</TooltipContent>
        </Tooltip>
        <Button
          v-if="running"
          variant="dialog"
          size="kira-lg"
          data-testid="generate-data-stop"
          @click="onStop"
        >
          {{ t('common.action.stop.label') }}
        </Button>
        <template v-else>
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="generate-data-cancel"
            @click="onClose"
          >
            {{ t('common.action.cancel.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="generate-data-submit"
            :disabled="rowCount < 1 || noUsableColumns || (!!tabId && gridViewStore.isStale(tabId))"
            @click="onGenerate"
          >
            {{ t('studio.generateData.submit.label') }}
          </Button>
        </template>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
