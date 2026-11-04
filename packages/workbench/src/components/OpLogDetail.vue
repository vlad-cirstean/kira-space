<script setup lang="ts" generic="R extends OpLogRecord">
import type { EditorLanguageId } from '@shared/domain/editor';
import type { OpLogRecord } from '@shared/domain/ops';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { useCopy } from '../behaviour/clipboard';
import { displayValue, formatClockTime, formatDateTime, formatDuration } from '../behaviour/format';
import {
  type OpLogDetailLine,
  type OpLogField,
  type OpLogStep,
  STATUS_ICON,
  statusLabel,
} from '../lib/opLog';
import type { MenuItem } from '../state/contextMenu';
import OpLogCodeBlock from './OpLogCodeBlock.vue';

// The expanded body of one op row: fields, command, error, steps, extra lines and an action bar.
const props = defineProps<{
  record: R;
  fields: readonly OpLogField[];
  steps: readonly OpLogStep[];
  lines: readonly OpLogDetailLine[];
  commandLanguage: EditorLanguageId | null;
  /** The row's context-menu items; the enabled ones render as buttons. */
  actions: readonly MenuItem[];
}>();

const emit = defineEmits<{ escape: [] }>();
const { copy } = useCopy();

const actionItems = (): Extract<MenuItem, { type: 'item' }>[] =>
  props.actions.filter(
    (item): item is Extract<MenuItem, { type: 'item' }> => item.type === 'item' && !item.disabled,
  );

function stepDuration(step: OpLogStep): string {
  return step.durationMs === null ? '' : formatDuration(step.durationMs);
}
</script>

<template>
  <!-- biome-ignore lint/a11y/noStaticElementInteractions: keys bubble up from the controls and selectable text inside. -->
  <div
    class="flex flex-col gap-1.5 bg-elevated py-1.5 pr-2 pl-[52px] text-kira-sm select-text"
    data-testid="op-detail"
    :data-op-id="record.id"
    @keydown.esc.stop="emit('escape')"
    @keydown.up.stop
    @keydown.down.stop
    @keydown.left.stop
    @keydown.right.stop
    @keydown.home.stop
    @keydown.end.stop
  >
    <dl class="m-0 grid grid-cols-[max-content_1fr] gap-x-3 gap-y-0.5">
      <dt class="text-muted-foreground">{{ t('common.field.time.label') }}</dt>
      <dd class="m-0 font-data">
        {{
          t('common.dotPair.label', {
            first: formatDateTime(record.startedAt),
            second: formatClockTime(record.startedAt),
          })
        }}
      </dd>
      <dt class="text-muted-foreground">{{ t('common.field.duration.label') }}</dt>
      <dd class="m-0 font-data">{{
        displayValue(record.durationMs === null ? null : formatDuration(record.durationMs))
      }}</dd>
      <template v-for="field in fields" :key="field.label">
        <dt class="text-muted-foreground">{{ field.label }}</dt>
        <dd :class="cn('m-0 min-w-0 break-all', field.code && 'font-data')">{{ field.value }}</dd>
      </template>
    </dl>

    <section v-if="record.command" class="flex flex-col gap-0.5">
      <div class="flex items-center gap-1 text-muted-foreground">
        <span>{{ t('common.field.command.label') }}</span>
        <TooltipIconButton
          icon="copy"
          :label="t('common.copyCommand.label')"
          size="inline"
          :icon-size="12"
          @click="
            copy(record.command ?? '', { message: t('workbench.opLog.copiedCommand.announce') })
          "
        />
      </div>
      <OpLogCodeBlock
        :text="record.command"
        :language="commandLanguage"
        data-testid="op-detail-command"
      />
    </section>

    <section v-if="record.error" class="flex flex-col gap-0.5">
      <div class="flex items-center gap-1 text-muted-foreground">
        <span>{{ t('workbench.opLog.error.status') }}</span>
        <TooltipIconButton
          icon="copy"
          :label="t('workbench.opLog.copyError.label')"
          size="inline"
          :icon-size="12"
          @click="copy(record.error ?? '', { message: t('workbench.opLog.copiedError.announce') })"
        />
      </div>
      <OpLogCodeBlock :text="record.error" tone="error" data-testid="op-detail-error" />
    </section>

    <ul v-if="steps.length > 0" class="m-0 flex list-none flex-col gap-0.5 p-0">
      <li
        v-for="(step, i) in steps"
        :key="i"
        class="flex min-w-0 flex-col gap-0.5"
        data-testid="op-detail-line"
      >
        <div class="flex min-w-0 items-center gap-1.5">
          <CodiconIcon
            :name="STATUS_ICON[step.status].icon"
            :size="12"
            :class="STATUS_ICON[step.status].tone"
            :title="statusLabel(step.status)"
          />
          <div class="min-w-0 flex-1 font-data"><TruncatedText :text="step.argv" /></div>
          <span v-if="step.exitCode !== null" class="shrink-0 font-data text-muted-foreground">{{
            t('workbench.opLog.detail.exit.label', { code: step.exitCode })
          }}</span>
          <span class="shrink-0 font-data text-muted-foreground">{{ stepDuration(step) }}</span>
        </div>
        <OpLogCodeBlock
          v-if="step.status === 'error' && step.stderr"
          :text="step.stderr"
          tone="error"
          data-testid="op-detail-stderr"
        />
      </li>
    </ul>

    <span
      v-for="(line, i) in lines"
      :key="`line-${i}`"
      :class="
        cn(
          'block font-data break-all whitespace-pre-wrap',
          line.tone === 'error' ? 'text-error' : 'text-muted-foreground',
        )
      "
      data-testid="op-detail-line"
    >
      {{ line.text }}
    </span>

    <div v-if="actionItems().length > 0" class="flex flex-wrap items-center gap-1">
      <Button
        v-for="item in actionItems()"
        :key="item.id"
        variant="toolbar"
        size="status"
        :data-testid="`op-action-${item.id}`"
        @click="item.run()"
      >
        <CodiconIcon v-if="item.icon" :name="item.icon" :size="12" />
        {{ item.label }}
      </Button>
    </div>
  </div>
</template>
