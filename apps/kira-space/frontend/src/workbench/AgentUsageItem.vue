<script setup lang="ts">
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import { t } from '@theme/i18n';
import { TONE_TEXT } from '@theme/lib/statusTone';
import { useIntervalFn, useNow } from '@vueuse/core';
import { formatCompactDuration, formatDateTime, formatRelative } from '@workbench/behaviour/format';
import { TIMING } from '@workbench/behaviour/timing';
import StatusBarItem from '@workbench/components/StatusBarItem.vue';
import { computed } from 'vue';
import type { AgentUsageWindow } from '../bridge/index';
import { useAgentUsage } from '../state/agentUsage';

// P239: ADE status-bar readout of the agent 5-hour and weekly limits.
const WARN_AT = 80;
const ERROR_AT = 95;

const { data, isError } = useAgentUsage();
const now = useNow({ scheduler: (cb) => useIntervalFn(cb, TIMING.clockTickSlow) });

const windows = computed(() => {
  const d = data.value;
  if (d?.state !== 'ok') return [];
  return [
    {
      label: t('common.field.session.label'),
      long: t('space.settings.agentUsage.sessionWindow.label'),
      window: d.fiveHour,
    },
    {
      label: t('space.settings.agentUsage.weekly.label'),
      long: t('space.settings.agentUsage.weekly.label'),
      window: d.sevenDay,
    },
  ].filter(
    (w): w is { label: string; long: string; window: AgentUsageWindow } => w.window !== null,
  );
});

const text = computed(() => {
  if (isError.value && windows.value.length === 0)
    return t('space.settings.agentUsage.unavailable.status');
  if (windows.value.length === 0) return t('space.settings.agentUsage.unknown.status');
  return windows.value
    .map((w) =>
      t('common.progressPercent.label', {
        label: w.label,
        percent: Math.round(w.window.usedPercent),
      }),
    )
    .join(' · ');
});

function toneOf(percent: number): 'warn' | 'error' | undefined {
  if (percent >= ERROR_AT) return 'error';
  if (percent >= WARN_AT) return 'warn';
  return undefined;
}

const tone = computed(() => toneOf(Math.max(0, ...windows.value.map((w) => w.window.usedPercent))));

const rows = computed(() =>
  windows.value.map((w) => {
    const percent = toneOf(w.window.usedPercent);
    const left = formatCompactDuration(
      Math.max(0, Math.round((w.window.resetsAt - now.value.getTime()) / 60_000)) * 60_000,
    );
    return {
      label: w.long,
      value: t('space.settings.agentUsage.line.status', {
        percent: Math.round(w.window.usedPercent),
        when: formatDateTime(w.window.resetsAt),
        left,
      }),
      class: percent ? TONE_TEXT[percent] : undefined,
    };
  }),
);

const sourceLine = computed(() => {
  const d = data.value;
  if (d?.state !== 'ok') return '';
  const when = formatRelative(d.updatedAt, now.value.getTime());
  return t(
    d.source === 'run'
      ? 'space.settings.agentUsage.sourceRun.hint'
      : 'space.settings.agentUsage.sourceSession.hint',
    { when },
  );
});

const detail = computed(() => {
  if (isError.value && windows.value.length === 0)
    return t('space.settings.agentUsage.readFailed.error');
  return data.value?.state === 'waiting' ? t('space.settings.agentUsage.waiting.hint') : '';
});

const hint = computed(() => sourceLine.value || detail.value || undefined);
</script>

<template>
  <StatusBarItem icon="dashboard" :tone="tone" data-testid="agent-usage-status">
    <span data-testid="agent-usage-text">{{ text }}</span>
    <template #tip>
      <div data-testid="agent-usage-tooltip">
        <TooltipDetails
          :title="t('space.settings.agentUsage.limits.tooltip')"
          :rows="rows"
          :hint="hint"
        />
      </div>
    </template>
  </StatusBarItem>
</template>
