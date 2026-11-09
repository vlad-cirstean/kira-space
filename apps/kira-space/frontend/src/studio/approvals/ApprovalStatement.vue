<script setup lang="ts">
import { connectionKindSchema } from '@shared/domain/connection';
import type { SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import { t } from '@theme/i18n';
import { computedAsync } from '@vueuse/core';
import { computed, ref } from 'vue';
import { canFormatConsole, consoleLanguageFor, formatConsoleText } from '../views/console/format';
import { sqlDialectFor } from '../views/shared/sqlIdent';
import { INLINE_CHARS } from '../workers/parse/client';
import { useParseWorker } from '../workers/parse/useParseWorker';
import FlushEditor from './FlushEditor.vue';
import TabbedContent from './TabbedContent.vue';

const props = defineProps<{ kind: string; statement: string }>();

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
  <TabbedContent
    v-model="view"
    :tabs="canToggle"
    :items="items"
    :label="t('studio.dbMcp.approval.statementView.aria')"
  >
    <FlushEditor
      :doc="doc"
      :language="consoleLanguageFor(connectionKind)"
      :sql-dialect="sqlDialectFor(connectionKind)"
      data-testid="db-mcp-approval-statement"
    />
  </TabbedContent>
</template>
