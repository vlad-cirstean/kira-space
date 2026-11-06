import type {
  ProofCartesianChart,
  ProofChartPayload,
  ProofChartViewState,
  ProofPieChart,
} from '@shared/domain/proof';
import { formatDateTime, formatWeekdayDate } from '@theme/format';
import { formatCount } from '@theme/i18n/number';
import type { EChartsOption } from 'echarts';
import { format } from 'echarts/core';
import type { ChartTheme } from '../charts/theme';

/** Agent strings reach ECharts' tooltip as HTML; every one of them passes this. */
const esc = format.encodeHTML;

export const TOOLTIP_CLASS = 'kira-proof-chart-tooltip';

// Above this many points a line or area drops its symbols (and with them point selection).
const SYMBOL_LIMIT = 1000;
const SYMBOLS_SELECTED = 10;
const TIME_SHORT_SPAN_MS = 48 * 3_600_000;

function withUnit(value: number, unit: string): string {
  const n = formatCount(value);
  return unit ? `${n} ${unit}` : n;
}

/** Category labels in order of first appearance across the series. */
function categoriesOf(payload: ProofChartPayload): string[] {
  if (payload.type === 'pie' || payload.x.type !== 'category') return [];
  const seen = new Set<string>();
  for (const s of payload.series) for (const p of s.points) seen.add(String(p[0]));
  return [...seen];
}

function timeSpan(chart: ProofCartesianChart): number {
  let lo = Number.POSITIVE_INFINITY;
  let hi = Number.NEGATIVE_INFINITY;
  for (const s of chart.series) {
    for (const p of s.points) {
      const x = Number(p[0]);
      if (x < lo) lo = x;
      if (x > hi) hi = x;
    }
  }
  return hi - lo;
}

function totalPoints(chart: ProofCartesianChart): number {
  return chart.series.reduce((n, s) => n + s.points.length, 0);
}

interface Axes {
  x: Record<string, unknown>;
  y: Record<string, unknown>;
}

function axisStyle(c: ChartTheme) {
  return {
    axisLine: { lineStyle: { color: c.axisLine } },
    axisTick: { show: false },
    splitLine: { lineStyle: { color: c.splitLine } },
    nameTextStyle: { color: c.mutedText },
  };
}

function axes(chart: ProofCartesianChart, c: ChartTheme, categories: string[]): Axes {
  const style = axisStyle(c);
  const labelBase = { color: c.mutedText, hideOverlap: true };
  const long = timeSpan(chart) > TIME_SHORT_SPAN_MS;
  const name = (n: string, location: 'middle' | 'end') =>
    n ? { name: n, nameLocation: location, nameGap: 28 } : {};
  const category: Record<string, unknown> = {
    ...style,
    type: 'category',
    data: categories,
    splitLine: { show: false },
    axisLabel: labelBase,
    ...name(chart.x.name, 'middle'),
  };
  const xOnly: Record<string, unknown> =
    chart.x.type === 'category'
      ? category
      : chart.x.type === 'time'
        ? {
            ...style,
            type: 'time',
            axisLabel: {
              ...labelBase,
              formatter: (v: number) => (long ? formatWeekdayDate(v) : formatDateTime(v)),
            },
            splitLine: { show: false },
            ...name(chart.x.name, 'middle'),
          }
        : {
            ...style,
            type: 'value',
            scale: true,
            splitLine: { show: false },
            axisLabel: { ...labelBase, formatter: (v: number) => formatCount(v) },
            ...name(chart.x.name, 'middle'),
          };
  const value: Record<string, unknown> = {
    ...style,
    type: 'value',
    axisLabel: { ...labelBase, formatter: (v: number) => withUnit(v, chart.y.unit) },
    ...name(chart.y.name, 'end'),
  };
  if (chart.horizontal) {
    // The category axis runs down, the value axis across.
    return {
      x: { ...value, ...name(chart.y.name, 'middle') },
      y: { ...xOnly, inverse: true, ...name(chart.x.name, 'end') },
    };
  }
  return { x: xOnly, y: value };
}

type Cell = number | null;

/** One series' values aligned to `categories`: null where the series has no point. */
function alignedValues(
  points: ProofCartesianChart['series'][number]['points'],
  categories: string[],
): Cell[] {
  const byCategory = new Map(points.map((p) => [String(p[0]), p[1]]));
  return categories.map((cat) => byCategory.get(cat) ?? null);
}

function selectedStyle(c: ChartTheme) {
  return { borderColor: c.selected, borderWidth: 2 };
}

function cartesianSeries(
  chart: ProofCartesianChart,
  c: ChartTheme,
  categories: string[],
  state: ProofChartViewState,
): Record<string, unknown>[] {
  const symbols = totalPoints(chart) <= SYMBOL_LIMIT;
  return chart.series.map((s, si) => {
    const isCategory = chart.x.type === 'category';
    const raw: unknown[] = isCategory
      ? alignedValues(s.points, categories)
      : s.points.map((p) => [p[0], p[1]]);
    const pick = state.selected && state.selected.series === si ? state.selected.index : -1;
    const data = raw.map((v, i) => {
      const item = horizontalPair(chart, v, isCategory, categories, i);
      if (i !== pick) return item;
      if (chart.type === 'bar') return { value: item, itemStyle: selectedStyle(c) };
      if (!symbols && chart.type !== 'scatter') return item;
      return { value: item, symbolSize: SYMBOLS_SELECTED, itemStyle: selectedStyle(c) };
    });
    const base = { name: s.name, data };
    const stack = chart.stacked ? { stack: 'total' } : {};
    switch (chart.type) {
      case 'bar':
        return { ...base, type: 'bar', ...stack, emphasis: { focus: 'series' } };
      case 'scatter':
        return { ...base, type: 'scatter', symbolSize: 6 };
      default:
        return {
          ...base,
          type: 'line',
          ...stack,
          connectNulls: false,
          showSymbol: symbols,
          symbolSize: 4,
          ...(chart.type === 'area' ? { areaStyle: { opacity: chart.stacked ? 0.6 : 0.25 } } : {}),
        };
    }
  });
}

/** Category data is a flat value list; a horizontal bar needs the value on x and the category on y. */
function horizontalPair(
  chart: ProofCartesianChart,
  v: unknown,
  isCategory: boolean,
  categories: string[],
  i: number,
): unknown {
  if (!chart.horizontal) return v;
  if (isCategory) return [v, categories[i]];
  const pair = v as [unknown, unknown];
  return [pair[1], pair[0]];
}

function axisHeader(chart: ProofCartesianChart, axisValue: unknown): string {
  if (chart.x.type === 'time') return esc(formatDateTime(Number(axisValue)));
  if (chart.x.type === 'value') return esc(formatCount(Number(axisValue)));
  return esc(String(axisValue));
}

interface TooltipParam {
  seriesName?: string;
  marker?: string;
  value?: unknown;
  axisValue?: unknown;
  name?: string;
  percent?: number;
}

function yOf(chart: ProofCartesianChart, p: TooltipParam): Cell {
  const v = p.value;
  if (!Array.isArray(v)) return typeof v === 'number' ? v : null;
  const y = chart.horizontal ? v[0] : v[1];
  return typeof y === 'number' ? y : null;
}

function cartesianTooltip(chart: ProofCartesianChart) {
  const row = (p: TooltipParam) => {
    const y = yOf(chart, p);
    return `${p.marker ?? ''}${esc(p.seriesName ?? '')}: ${y === null ? '–' : esc(withUnit(y, chart.y.unit))}`;
  };
  return (raw: unknown): string => {
    const params = (Array.isArray(raw) ? raw : [raw]) as TooltipParam[];
    if (chart.type === 'scatter') {
      const p = params[0];
      const v = Array.isArray(p?.value) ? p.value : [];
      const xs = esc(formatCount(Number(v[chart.horizontal ? 1 : 0])));
      const label = chart.x.name ? `${esc(chart.x.name)}: ` : '';
      return `${row(p ?? {})}<br/>${label}${xs}`;
    }
    const first = params[0];
    const header = axisHeader(chart, first?.axisValue ?? first?.name);
    return `<div style="font-weight:600">${header}</div>${params.map(row).join('<br/>')}`;
  };
}

function cartesianOption(
  chart: ProofCartesianChart,
  c: ChartTheme,
  state: ProofChartViewState,
): Record<string, unknown> {
  const categories = categoriesOf(chart);
  const { x, y } = axes(chart, c, categories);
  const zoom = state.zoom ?? { start: 0, end: 100 };
  const zoomAxis = chart.horizontal ? { yAxisIndex: 0 } : { xAxisIndex: 0 };
  return {
    grid: { left: 16, right: 24, top: 44, bottom: 16, ...GRID_BOUNDS },
    xAxis: x,
    yAxis: y,
    series: cartesianSeries(chart, c, categories, state),
    dataZoom: [
      { type: 'inside', ...zoomAxis, filterMode: 'none', start: zoom.start, end: zoom.end },
    ],
    tooltip: {
      ...tooltipBase(c),
      trigger: chart.type === 'scatter' ? 'item' : 'axis',
      axisPointer: { type: chart.type === 'bar' ? 'shadow' : 'line' },
      formatter: cartesianTooltip(chart),
    },
  };
}

const GRID_BOUNDS = { outerBoundsMode: 'same', outerBoundsContain: 'all' } as const;

function tooltipBase(c: ChartTheme) {
  return {
    className: TOOLTIP_CLASS,
    confine: true,
    backgroundColor: c.tooltipBackground,
    borderColor: c.tooltipBorder,
    textStyle: { color: c.tooltipText },
  };
}

function pieOption(
  chart: ProofPieChart,
  c: ChartTheme,
  state: ProofChartViewState,
): Record<string, unknown> {
  const pick = state.selected && state.selected.series === 0 ? state.selected.index : -1;
  return {
    tooltip: {
      ...tooltipBase(c),
      trigger: 'item',
      formatter: (raw: TooltipParam) =>
        `${raw.marker ?? ''}${esc(raw.name ?? '')}: ${esc(withUnit(Number(raw.value), chart.unit))} (${raw.percent}%)`,
    },
    series: [
      {
        type: 'pie',
        radius: '70%',
        center: ['50%', '55%'],
        selectedMode: 'single',
        selectedOffset: 8,
        label: {
          color: c.text,
          formatter: (p: { name: string; percent: number }) => `${p.name}: ${p.percent}%`,
        },
        labelLine: { lineStyle: { color: c.axisLine } },
        itemStyle: { borderColor: c.tooltipBackground, borderWidth: 1 },
        data: chart.slices.map((s, i) => ({ name: s.name, value: s.value, selected: i === pick })),
      },
    ],
  };
}

/** The ECharts option for a stored chart payload. Selection, hidden series and zoom are built in,
 *  so the first render already shows them. */
export function chartOption(
  payload: ProofChartPayload,
  theme: ChartTheme,
  state: ProofChartViewState,
): EChartsOption {
  const multi = payload.type === 'pie' || payload.series.length > 1;
  const hidden = new Set(state.hidden);
  const names =
    payload.type === 'pie' ? payload.slices.map((s) => s.name) : payload.series.map((s) => s.name);
  const body =
    payload.type === 'pie'
      ? pieOption(payload, theme, state)
      : cartesianOption(payload, theme, state);
  return {
    animation: false,
    color: theme.series,
    textStyle: { fontFamily: theme.fontFamily, color: theme.mutedText },
    aria: { enabled: true },
    legend: {
      show: multi,
      type: 'scroll',
      top: 0,
      textStyle: { color: theme.text },
      inactiveColor: theme.mutedText,
      inactiveBorderColor: theme.mutedText,
      pageIconColor: theme.text,
      pageTextStyle: { color: theme.mutedText },
      selected: Object.fromEntries(names.map((n) => [n, !hidden.has(n)])),
    },
    ...body,
  } as EChartsOption;
}
