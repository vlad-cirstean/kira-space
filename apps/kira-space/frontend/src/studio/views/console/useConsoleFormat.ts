import type { ConnectionKind } from '@shared/domain/connection';
import {
  type SplitSqlOptions,
  type SqlStatement,
  splitSqlStatements,
  statementAtOffset,
  trimmedStatementStart,
} from '@shared/domain/sqlSplit';
import { t } from '@theme/i18n';
import { type ComputedRef, nextTick, type Ref, ref } from 'vue';
import type { ConsoleTabRecord } from '../../state/tabDomain';
import { INLINE_CHARS } from '../../workers/parse/client';
import { useParseWorker } from '../../workers/parse/useParseWorker';
import { reasonText } from '../shared/reasonText';
import type { SqlDialect } from '../shared/sqlIdent';
import { formatConsoleText } from './format';
import { setText } from './state';

interface EditorHandle {
  setCursor: (pos: number) => void;
}

export function useConsoleFormat(opts: {
  tab: Ref<ConsoleTabRecord>;
  connectionKind: ComputedRef<ConnectionKind | undefined>;
  canFormat: ComputedRef<boolean>;
  dialect: ComputedRef<SqlDialect | undefined>;
  splitOptionsFor: (d: SqlDialect | undefined) => SplitSqlOptions;
  cursorPos: Ref<number>;
  editorHost: Ref<EditorHandle | null>;
  resetStalePreviewState: () => void;
}) {
  const parse = useParseWorker();
  const formatError = ref<string | null>(null);
  const formatWarning = ref<string | null>(null);
  const formatNote = ref<string | null>(null);
  const formatting = ref(false);

  function clear(): void {
    formatError.value = null;
    formatWarning.value = null;
    formatNote.value = null;
  }

  type FormatOutcome = Awaited<ReturnType<typeof formatConsoleText>>;

  function restoreCursor(text: string, splitOptions: SplitSqlOptions, beforeIndex: number): void {
    if (beforeIndex < 0) return;
    const target = splitSqlStatements(text, splitOptions)[beforeIndex];
    // `target.start` is the raw boundary, still covering the blank line the formatter's joiner
    // inserts; statementAtOffset attributes that gap to the preceding statement. Skip past it so
    // Run statement resolves to this one.
    const target0 = target ? trimmedStatementStart(text, target) : 0;
    void nextTick().then(() => opts.editorHost.value?.setCursor(target0));
  }

  function reportOutcome(
    result: FormatOutcome,
    count: number,
    originalText: string,
    kind: ConnectionKind,
  ): void {
    const first = result.failures[0];
    if (first) {
      formatWarning.value = t('studio.console.format.partial.error', {
        formatted: count - result.failures.length,
        count,
        index: first.index + 1,
        reason: reasonText(first.reason),
      });
    } else if (result.text === originalText) {
      formatNote.value =
        kind === 'clickhouse'
          ? t('studio.console.format.alreadyClickhouse.body')
          : t('studio.console.format.already.body');
    }
  }

  function onFormat(): void {
    const kind = opts.connectionKind.value;
    if (!kind || !opts.canFormat.value || formatting.value) return;
    const splitOptions = opts.splitOptionsFor(opts.dialect.value);
    const originalText = opts.tab.value.state.text;
    const before = splitSqlStatements(originalText, splitOptions);
    // Same ownership rule Run/Explain use, so Format and Run agree on the caret's statement.
    const beforeIndex = before.indexOf(
      statementAtOffset(before, originalText, opts.cursorPos.value) as SqlStatement,
    );
    formatting.value = true;
    void (async () => {
      // Over the inline threshold the whole pass runs in the shared parse worker: a pasted
      // multi-hundred-KB dump would otherwise block the main thread.
      let result: FormatOutcome;
      try {
        result =
          originalText.length <= INLINE_CHARS
            ? await formatConsoleText(kind, originalText)
            : await parse.run('console.format', { kind, text: originalText });
      } catch (e) {
        // AbortError: the view unmounted mid-run.
        if (!(e instanceof DOMException && e.name === 'AbortError')) {
          formatError.value =
            e instanceof Error ? e.message : t('studio.console.format.failed.error');
        }
        return;
      } finally {
        formatting.value = false;
      }
      if (opts.tab.value.state.text !== originalText) {
        formatNote.value = t('studio.console.format.stale.body');
        return;
      }
      opts.resetStalePreviewState();
      if (!result.ok) {
        formatError.value = result.reason
          ? reasonText(result.reason)
          : t('studio.console.format.failed.error');
        return;
      }
      setText(opts.tab.value.id, result.text);
      restoreCursor(result.text, splitOptions, beforeIndex);
      void nextTick().then(() => reportOutcome(result, before.length, originalText, kind));
    })();
  }

  return { formatError, formatWarning, formatNote, onFormat, clearFormatMessages: clear };
}
