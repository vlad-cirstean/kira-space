<script setup lang="ts">
import type { ForeignKeyMeta } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Table, TableBody, TableCell, TableRow } from '@theme/components/ui/table';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import PointPopover from '@workbench/components/PointPopover.vue';
import { onMounted, onUnmounted, ref } from 'vue';
import { control } from '../../bridge/control';
import { typeClassTextClass } from '../../theme/icons';
import { type FkPreviewState, fetchReferencedRow, type PreviewSignal } from './fkPreview';
import { type FkNavContext, foreignKeyValueFilter, qualifiedNameForPath } from './menu';

// Anchored to the click point through PointPopover, NOT the nav button element. That
// button is imperative DOM, destroyed by SlickGrid's own `invalidateRow` on the next scroll frame,
// so a detached reference is exactly what this component would hold if it anchored to it instead.
const props = defineProps<{
  /** The click point, in viewport coordinates. */
  x: number;
  y: number;
  entry: ForeignKeyMeta;
  ctx: FkNavContext;
  /** Attribution only (fkPreview.ts's own fetchReferencedRow) — never the target's tab id. */
  sourceTabId: string;
  /** SlickGridHost's own resolved nav MenuItem.run for this edge — "Open in new tab" is that
   *  exact click behaviour (navigateForeignKey), unchanged, one click further in (§2 item 3). Not
   *  called directly from here so navigateForeignKey itself stays untouched (§10.3's checklist). */
  openInNewTab: () => void;
}>();

const emit = defineEmits<{ close: [] }>();

const state = ref<FkPreviewState>({ status: 'loading' });
const signal: PreviewSignal = { cancelled: false, opId: null };

const tableLabel = qualifiedNameForPath(props.ctx.connectionId, props.entry.referencedPath);

async function load(): Promise<void> {
  const filter = foreignKeyValueFilter(
    props.ctx.dialect,
    props.entry.columns,
    props.entry.referencedColumns,
    props.ctx.rowValues,
    props.ctx.numericColumns,
  );
  // Defensive only: the caller (SlickGridHost's onGridClick) never opens this popover for an edge
  // whose filter can't be built (cellNavEntry already filters those out) — this can't happen on a
  // real click, but a filter-less fetch would be meaningless if it somehow did.
  if (filter === null) {
    state.value = { status: 'error', message: t('studio.grid.fkPreview.noValue.error') };
    return;
  }
  state.value = { status: 'loading' };
  const result = await fetchReferencedRow(
    props.ctx.connectionId,
    props.sourceTabId,
    props.entry,
    filter,
    signal,
  );
  if (signal.cancelled) return;
  state.value = result;
}

function onOpenChange(open: boolean): void {
  if (!open) emit('close');
}

function onOpenClick(): void {
  emit('close');
  props.openInNewTab();
}

onMounted(() => {
  void load();
});

onUnmounted(() => {
  // Belt and braces (§4.1): cancel is best-effort server-side, and `signal.cancelled` alone
  // already drops a response that lands after this — cancelling too costs nothing when the read
  // has already finished.
  if (signal.opId && state.value.status === 'loading') {
    signal.cancelled = true;
    fireAndForget(control.opsCancel(signal.opId), 'cancelFkPreview');
  }
});
</script>

<template>
  <PointPopover
    :x="x"
    :y="y"
    :open="true"
    class="w-80 gap-0 overflow-hidden p-0 max-h-(--reka-popover-content-available-height)"
    @update:open="onOpenChange"
  >
    <div class="flex min-h-0 flex-col" data-testid="fk-preview">
      <div class="flex items-center justify-between border-b border-border flex-none gap-1 p-1.5">
        <span class="min-w-0 font-medium"><TruncatedText :text="tableLabel" /></span>
        <Badge
          v-if="state.status === 'ready' && state.hasMore"
          variant="info"
          data-testid="fk-preview-more"
        >
          {{ t('studio.grid.fkPreview.manyRows.status', { count: state.rows.length }) }}
        </Badge>
      </div>

      <div class="flex border-b border-border flex-none gap-1 p-1.5">
        <Button variant="toolbar" size="kira" data-testid="fk-preview-open" @click="onOpenClick">
          <CodiconIcon name="arrow-right" :size="13" />
          {{ t('common.action.openInNewTab.label') }}
        </Button>
      </div>

      <!-- flex-1 min-h-0 is load-bearing: without it a flex child refuses to shrink below its
           content height, and the panel overflows its own max-height instead of scrolling here. -->
      <div class="flex-1 min-h-0 overflow-y-auto p-1.5">
        <InlineState v-if="state.status === 'loading'" kind="loading" />
        <InlineState v-else-if="state.status === 'error'" kind="error">{{
          state.message
        }}</InlineState>
        <Alert
          v-else-if="state.status === 'ready' && state.rows.length === 0"
          variant="info"
          data-testid="fk-preview-empty"
        >
          <AlertDescription>{{
            t('studio.grid.fkPreview.noMatch.empty', { table: tableLabel })
          }}</AlertDescription>
        </Alert>
        <!-- `header-key` marker; look matches SlickGridHost header badges. -->
        <Table v-else-if="state.status === 'ready'" columns>
          <TableBody>
            <TableRow v-for="(col, i) in state.columns" :key="col.name">
              <TableCell
                class="whitespace-nowrap align-top font-medium"
                :class="typeClassTextClass(col.typeClass)"
              >
                {{ col.name }}
                <span v-if="col.isPrimaryKey" class="header-key ml-1 shrink-0 text-warn">{{
                  t('studio.definition.primaryKey.label')
                }}</span>
                <span v-if="col.isTarget" class="header-key ml-1 shrink-0 text-info">{{
                  t('studio.definition.foreignKey.label')
                }}</span>
              </TableCell>
              <TableCell class="break-words">
                <Badge
                  v-if="state.rows[0]?.values[i]?.isNull"
                  variant="info"
                  data-testid="fk-preview-null"
                  >{{
                    t('studio.common.nullValue.label')
                  }}</Badge
                >
                <span v-else>{{ state.rows[0]?.values[i]?.text }}</span>
                <Badge v-if="state.rows[0]?.values[i]?.truncated" variant="warn">{{
                  t('studio.common.truncated.status')
                }}</Badge>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </div>
  </PointPopover>
</template>
