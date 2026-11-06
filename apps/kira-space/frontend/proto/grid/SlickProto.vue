<script setup lang="ts">
import {
  type Column,
  type CustomDataView,
  type FormatterResultWithText,
  SlickGrid,
  SlickHybridSelectionModel,
} from 'slickgrid';
import 'slickgrid/dist/styles/css/slick.grid.css';
import { defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue';
import { categoryForTypeClass } from '../../src/theme/icons';
import {
  alignmentFor,
  GUTTER_WIDTH,
  headerAwareMinWidth,
  initialWidthsByIndex,
} from '../../src/views/shared/page/columns';
import { createGridDataSource, type RowHandle } from '../../src/views/shared/slick/dataSource';
import { gutterColumn } from '../../src/views/shared/slick/gridHostShared';
import { KiraSlickGrid } from '../../src/views/shared/slick/kiraSlickGrid';
import '../../src/views/shared/slick/slickTheme.css';
import { createProtoData, GUTTER_FIELD, readParams } from './data';
import { createState } from './state';
import { readPalette } from './theme';
import { createTrace, slickSource } from './trace';

// biome-ignore lint/suspicious/noExplicitAny: same escape hatch SlickGridHost.vue uses.
type KiraColumn = Column<any>;

const query = new URLSearchParams(window.location.search);
const variant = query.get('variant') === 'stock' ? 'stock' : 'kira';
const params = readParams(window.location.search);
const data = createProtoData(params.fixture);
const state = createState(data, params, readPalette(params.rowHeight ?? undefined));
const rowHeight = state.palette.rowHeight;

const Hud = __KIRA_DEBUG_HOOKS__ ? defineAsyncComponent(() => import('./Hud.vue')) : null;
const mountRef = ref<HTMLElement | null>(null);
let grid: SlickGrid<RowHandle, KiraColumn> | null = null;

function formatter(_row: number, _cell: number, value: unknown): string | FormatterResultWithText {
  const view = value as { text: string; isNull: boolean; truncated: boolean; masked?: boolean };
  if (view.isNull) return { text: 'NULL', addClasses: 'cell-null' };
  const masked = view.masked ? 'cell-masked' : '';
  if (view.truncated) {
    return {
      text: view.text,
      addClasses: masked ? `cell-truncated ${masked}` : 'cell-truncated',
      toolTip: 'value truncated at 64 KB',
    };
  }
  return masked ? { text: view.text, addClasses: masked } : view.text;
}

function gutterFormatter(
  _row: number,
  _cell: number,
  _value: unknown,
  _col: unknown,
  item: RowHandle,
): string {
  return String(item.row + 1);
}

function buildColumns(): KiraColumn[] {
  const widths = initialWidthsByIndex(data.page);
  const columns: KiraColumn[] =
    variant === 'kira'
      ? [gutterColumn(GUTTER_FIELD, gutterFormatter, 'grid-gutter-cell')]
      : [
          {
            id: GUTTER_FIELD,
            field: GUTTER_FIELD,
            name: '',
            width: GUTTER_WIDTH,
            resizable: false,
            formatter: gutterFormatter,
          },
        ];
  data.columns.forEach((descriptor, index) => {
    const floor = headerAwareMinWidth(descriptor.name, {
      padding: 16,
      sortControl: 16,
      keyBadge: 0,
    });
    const classes = [`tc-${categoryForTypeClass(descriptor.typeClass)}`];
    if (alignmentFor(descriptor) === 'right') classes.push('kira-align-right');
    columns.push({
      id: descriptor.name,
      field: descriptor.name,
      name: descriptor.name,
      width: Math.max(floor, widths[index] ?? 96),
      minWidth: floor,
      resizable: true,
      sortable: variant === 'kira',
      ...(variant === 'kira' ? { cssClass: classes.join(' '), formatter } : {}),
    });
  });
  return columns;
}

onMounted(() => {
  const el = mountRef.value;
  if (!el) return;
  const names = data.columns.map((c) => c.name);
  const source = createGridDataSource({
    index: { displayRows: null, pageRowCount: data.rowCount },
    inserts: [],
    extractValue: (item, field) => {
      const view = data.viewAt(item.row, names.indexOf(field));
      return variant === 'kira' ? view : (view.text as never);
    },
  });
  const options = {
    rowHeight,
    enableColumnReorder: false,
    enableHtmlRendering: false,
    autosizeColsMode: 'LegacyOff' as const,
    frozenColumn: 0,
    enableMouseWheelScrollHandler: false,
    enableCellNavigation: true,
    explicitInitialization: true,
    dataItemColumnValueExtractor: (item: RowHandle, column: KiraColumn) =>
      source.extractValue(item, String(column.field)),
    ...(variant === 'kira'
      ? {
          selectedCellCssClass: 'kira-cell-selected',
          multiSelect: true,
          tristateMultiColumnSort: true,
          multiColumnSort: false,
          numberedMultiColumnSort: true,
          sortColNumberInSeparateSpan: true,
        }
      : {}),
  };
  const Grid = variant === 'kira' ? KiraSlickGrid : SlickGrid;
  grid = new Grid(el, source as CustomDataView<RowHandle>, buildColumns(), options) as SlickGrid<
    RowHandle,
    KiraColumn
  >;
  if (variant === 'kira') {
    grid.setSelectionModel(
      new SlickHybridSelectionModel({
        selectionType: 'mixed',
        rowSelectColumnIds: [GUTTER_FIELD],
        selectActiveCell: true,
        selectActiveRow: true,
        dragToSelect: true,
        autoScrollWhenDrag: true,
        enableMultiSelection: false,
        showDragHandle: false,
      }),
    );
  }
  grid.init();
  grid.render();
  if (__KIRA_DEBUG_HOOKS__) window.__kiraProtoTrace = createTrace(slickSource(el, rowHeight));
});

onBeforeUnmount(() => grid?.destroy());
</script>

<template>
  <div
    class="slick-grid-host kira-grid--row-coloring bg-bg text-fg"
    data-testid="data-grid"
    :style="{
      width: params.width ? `${params.width}px` : '100vw',
      height: params.height ? `${params.height}px` : '100vh',
    }"
  >
    <div ref="mountRef" class="slick-grid-mount"></div>
    <component :is="Hud" v-if="Hud" />
  </div>
</template>
