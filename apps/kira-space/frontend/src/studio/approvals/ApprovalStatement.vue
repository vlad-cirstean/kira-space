<script setup lang="ts">
import { connectionKindSchema } from '@shared/domain/connection';
import SecondaryTabs, { type SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import { t } from '@theme/i18n';
import { computedAsync } from '@vueuse/core';
import { computed, ref } from 'vue';
import MonacoHost from '../editor/MonacoHost.vue';
import { canFormatConsole, consoleLanguageFor, formatConsoleText } from '../views/console/format';
import { sqlDialectFor } from '../views/shared/sqlIdent';
import { INLINE_CHARS } from '../workers/parse/client';
import { useParseWorker } from '../workers/parse/useParseWorker';

const props = defineProps<{ kind: string; statement: string; compact?: boolean }>();

const parse = useParseWorker();
const connectionKind = computed(() => connectionKindSchema.safeParse(props.kind).data);

// A failed format shows the statement as sent; it is not an error to surface.
const formatted = computedAsync<string | null>(async (onCancel) => {
  const kind = connectionKind.value;
  const statement = props.statement;
  if (!kind || !canFormatConsole(kind)) return statement;
  const ctrl = new AbortController();
  onCancel(() => ctrl.abort());
  try {
    const result =
      statement.length <= INLINE_CHARS
        ? await formatConsoleText(kind, statement)
        : await parse.run('console.format', { kind, text: statement }, { signal: ctrl.signal });
    return result.ok ? result.text : statement;
  } catch {
    return statement;
  }
}, null);

const view = ref<'formatted' | 'asSent'>('formatted');
const canToggle = computed(() => formatted.value !== null && formatted.value !== props.statement);
const doc = computed(() =>
  view.value === 'asSent' || !canToggle.value ? props.statement : (formatted.value ?? ''),
);

const items = computed<SecondaryTabItem[]>(() => [
  {
    value: 'formatted',
    label: t('studio.dbMcp.approval.formatted.option'),
    testid: 'db-mcp-approval-view-formatted',
  },
  {
    value: 'asSent',
    label: t('studio.dbMcp.approval.asSent.option'),
    testid: 'db-mcp-approval-view-as-sent',
  },
]);
</script>

<template>
  <div :class="compact ? 'flex shrink-0 flex-col gap-1.5' : 'flex min-h-0 flex-1 flex-col gap-1.5'">
    <SecondaryTabs
      v-if="canToggle"
      v-model="view"
      class="self-start"
      variant="segmented"
      :items="items"
      :aria-label="t('studio.dbMcp.approval.statementView.aria')"
    />
    <div
      :class="
        compact
          ? 'h-24 overflow-hidden rounded-kira'
          : 'min-h-40 flex-1 overflow-hidden rounded-kira'
      "
      data-testid="db-mcp-approval-statement"
    >
      <MonacoHost
        :doc="doc"
        :language="consoleLanguageFor(connectionKind)"
        :sql-dialect="sqlDialectFor(connectionKind)"
        readonly
        force-wrap
      />
    </div>
  </div>
</template>
