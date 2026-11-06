<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Card } from '@theme/components/ui/card';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatCount } from '@workbench/behaviour/format';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { computed, reactive, ref } from 'vue';
import MonacoHost from '../../editor/MonacoHost.vue';
import { useStudioSettingsStore } from '../../state/settings';
import { getPlan } from './explainResults';
import type { PlanNode } from './planModel';

// The plan-result sibling of ConsoleResultGrid.vue, mounted instead of it when
// the active result set's `kind` is 'plan' (ConsoleView.vue). Reads its own data from
// explainResults.ts by `pageKey` — same "one prop, resolve everything else from a store" shape
// ConsoleResultGrid.vue's own `pageKey` already uses.
const props = defineProps<{ pageKey: string }>();
const settingsStore = useStudioSettingsStore();

const result = computed(() => getPlan(props.pageKey));
const plan = computed(() => result.value?.plan);

const NATIVE_COST_KEY: Record<string, MessageKey> = {
  'postgres-planner': 'studio.console.plan.cost.postgres.label',
  'mysql-cost': 'studio.console.plan.cost.mysql.label',
  'mariadb-cost': 'studio.console.plan.cost.mariadb.label',
};
const nativeCostLabel = computed(() => {
  const cost = plan.value?.nativeCost;
  if (!cost) return null;
  return t(NATIVE_COST_KEY[cost.unit] ?? 'common.cost.label', {
    value: formatCount(cost.value),
  });
});

// D14: the threshold is an estimated-rows-read number, never a cost unit (F17) — sqlite reports
// neither, so the verdict says the threshold doesn't apply here rather than showing a silent zero.
const verdict = computed(() => {
  const p = plan.value;
  if (!p) return '';
  if (p.estimatedRowsRead === undefined) {
    return p.kind === 'sqlite'
      ? t('studio.console.plan.sqliteNoEstimate.body')
      : t('studio.console.plan.noEstimate.body');
  }
  return p.overThreshold
    ? t('studio.console.plan.overThreshold.status', {
        count: p.estimatedRowsRead,
        threshold: formatCount(settingsStore.advanced.expensiveQueryRows),
      })
    : t('studio.console.plan.estimatedRows.status', { count: p.estimatedRowsRead });
});

// D17: "tens of nodes" (Postgres's own parallel-worker plans top out in the low hundreds) — not
// virtualized, and flattened once here rather than rendered by a recursive component, the same
// "flatten, don't recurse" shape DocumentTree.vue's own visibleLines() already takes.
interface FlatRow {
  node: PlanNode;
  depth: number;
  id: string;
  hasChildren: boolean;
}

const collapsedIds = reactive(new Set<string>());
function toggleNode(id: string): void {
  if (collapsedIds.has(id)) collapsedIds.delete(id);
  else collapsedIds.add(id);
}

const flatRows = computed<FlatRow[]>(() => {
  const root = plan.value?.root;
  if (!root) return [];
  const out: FlatRow[] = [];
  const walk = (node: PlanNode, depth: number, id: string): void => {
    out.push({ node, depth, id, hasChildren: node.children.length > 0 });
    if (collapsedIds.has(id)) return;
    node.children.forEach((child, i) => {
      walk(child, depth + 1, `${id}.${i}`);
    });
  };
  walk(root, 0, '0');
  return out;
});

function metricsLine(node: PlanNode): string {
  return node.metrics.map((m) => `${m.label}: ${m.value}`).join(' · ');
}

const showRaw = ref(false);
// Postgres/MySQL/MariaDB's raw text is one JSON document; ClickHouse's is a JSON plan plus an
// appended ESTIMATE block (explain.ts's own composed text, not valid JSON on its own), and
// SQLite's is reconstructed tab-separated rows — 'plain' avoids CodeMirror's JSON linter flagging
// either of the latter two as broken.
const rawLanguage = computed(() =>
  plan.value?.kind === 'postgres' || plan.value?.kind === 'mysql' || plan.value?.kind === 'mariadb'
    ? 'json'
    : 'plain',
);
</script>

<template>
  <div
    class="h-full overflow-auto flex flex-col text-kira-md gap-1.5 p-2"
    data-testid="explain-result-view"
  >
    <template v-if="plan && result">
      <div class="flex items-center flex-wrap gap-2">
        <div
          class="flex items-center gap-1"
          :class="plan.overThreshold ? 'text-warn' : 'text-ok'"
          data-testid="explain-verdict"
          :data-over-threshold="plan.overThreshold"
        >
          <CodiconIcon :name="plan.overThreshold ? 'warning' : 'check'" :size="12" />
          <span>{{ verdict }}</span>
        </div>
        <Tooltip v-if="nativeCostLabel">
          <TooltipTrigger as-child>
            <span :class="rowMeta" data-testid="explain-native-cost">{{ nativeCostLabel }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ t('studio.console.plan.nativeCost.tooltip') }}</TooltipContent>
        </Tooltip>
      </div>
      <p
        :class="[rowMeta, 'whitespace-pre-wrap break-words font-data']"
        data-testid="explain-statement"
        >{{
          result.statement
        }}</p
      >

      <ul
        v-if="plan.issues.length > 0"
        class="flex flex-col list-none m-0 p-0 gap-1"
        data-testid="explain-issues"
      >
        <li
          v-for="(issue, i) in plan.issues"
          :key="i"
          class="flex items-start gap-1"
          :class="issue.severity === 'warn' ? 'text-warn' : 'text-muted-foreground'"
          :data-severity="issue.severity"
        >
          <CodiconIcon :name="issue.severity === 'warn' ? 'warning' : 'info'" :size="12" />
          <span>{{ issue.message }}</span>
        </li>
      </ul>
      <InlineState v-else kind="empty" data-testid="explain-no-issues">{{
        t('studio.console.plan.noIssues.empty')
      }}</InlineState>

      <Card variant="list" data-testid="explain-tree">
        <div
          v-for="row in flatRows"
          :key="row.id"
          :class="
            cn(
              rowVariants({ layout: 'tree' }),
              'flex-wrap items-baseline gap-1.5 py-0.5 pr-1.5 whitespace-normal',
            )
          "
          data-testid="explain-plan-node"
          :style="{ paddingLeft: `${row.depth * 18 + 4}px` }"
        >
          <TreeTwisty
            :expanded="!collapsedIds.has(row.id)"
            :has-children="row.hasChildren"
            @toggle="toggleNode(row.id)"
          />
          <span class="font-data">{{ row.node.label }}</span>
          <span v-if="row.node.estimatedRows !== undefined" class="plan-meta" :class="rowMeta">{{
            t('studio.console.plan.nodeRows.label', { count: row.node.estimatedRows })
          }}</span>
          <span v-if="row.node.cost" class="plan-meta" :class="rowMeta">{{
            t('common.cost.label', { value: formatCount(row.node.cost.total) })
          }}</span>
          <span v-if="row.node.detail" class="text-subtle text-kira-sm font-data">{{
            row.node.detail
          }}</span>
          <span v-if="row.node.metrics.length" class="plan-metrics" :class="rowMeta">{{
            metricsLine(row.node)
          }}</span>
        </div>
      </Card>

      <div class="flex items-center gap-1">
        <TooltipIconButton
          icon="code"
          :label="t('studio.console.plan.raw.tooltip')"
          :aria-label="t('studio.console.plan.raw.aria')"
          :pressed="showRaw"
          data-testid="explain-raw-toggle"
          @click="showRaw = !showRaw"
        />
        <span :class="rowMeta">{{ t('common.raw.label') }}</span>
      </div>
      <div v-if="showRaw" class="h-64 rounded-kira overflow-hidden" data-testid="explain-raw">
        <MonacoHost :doc="plan.raw" :language="rawLanguage" readonly :autocomplete="false" />
      </div>
    </template>
    <InlineState v-else kind="empty">{{ t('studio.console.plan.empty.empty') }}</InlineState>
  </div>
  <!-- P110 I2-16: `.verdict.warn`/`.issue-list li[.warn/.info]`/`.plan-row:hover` all moved onto
       the template as ternaries/utilities -- `issue.severity` is exhaustively 'warn'/'info'
       (planModel.ts's own PlanIssue), so the <li> ternary needs no default branch. -->
</template>
