import { useMutationObserver } from '@vueuse/core';
import { ref } from 'vue';

export interface ChartTheme {
  text: string;
  mutedText: string;
  axisLine: string;
  splitLine: string;
  bar: string;
  barHover: string;
  tooltipBackground: string;
  tooltipBorder: string;
  tooltipText: string;
  brush: string;
  selected: string;
  fontFamily: string;
  series: string[];
}

function token(name: string): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return value || 'gray';
}

/** Chart colours from the app's CSS tokens, read now; recompute after a theme change. */
function chartTheme(): ChartTheme {
  return {
    text: token('--kira-fg'),
    mutedText: token('--kira-fg-muted'),
    axisLine: token('--kira-border'),
    splitLine: token('--kira-border'),
    bar: token('--kira-accent'),
    barHover: token('--kira-info'),
    tooltipBackground: token('--kira-bg-elevated'),
    tooltipBorder: token('--kira-border'),
    tooltipText: token('--kira-fg'),
    brush: token('--kira-select'),
    selected: token('--kira-fg'),
    fontFamily: getComputedStyle(document.body).fontFamily,
    series: [
      token('--kira-conn-blue'),
      token('--kira-ok'),
      token('--kira-warn'),
      token('--kira-error'),
      token('--kira-state-on'),
      token('--kira-var-resolved'),
      token('--kira-conn-teal'),
      token('--kira-conn-violet'),
      token('--kira-conn-orange'),
      token('--kira-conn-magenta'),
      token('--kira-conn-olive'),
      token('--kira-conn-cyan'),
    ],
  };
}

/** Reactive theme: refreshes when `<html>` changes its class or style (a theme switch). */
export function useChartTheme() {
  const theme = ref<ChartTheme>(chartTheme());
  useMutationObserver(
    document.documentElement,
    () => {
      theme.value = chartTheme();
    },
    { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] },
  );
  return theme;
}
