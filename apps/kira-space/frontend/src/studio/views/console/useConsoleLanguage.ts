import type { ConnectionKind } from '@shared/domain/connection';
import type { EditorLanguageId } from '@shared/domain/editor';
import { useQuery } from '@tanstack/vue-query';
import { computed, type Ref, watch } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import { containerPathFor, useSchemaColumnsStore } from '../../state/schemaColumns';
import { ddlSchemaFor, schemaQueryOptions } from '../../state/schemas';
import type { ConsoleTabRecord } from '../../state/tabDomain';
import { lexOptionsFor, type SqlDialect, sqlDialectFor } from '../shared/sqlIdent';
import { consoleCompletionSources } from './completion';
import { consoleLanguageFor } from './format';
import { consoleLintSource } from './lint';
import { sqlHoverSource } from './sqlHover';

/** The connection-derived editor setup of a console tab: dialect, language, completion, lint, hover. */
export function useConsoleLanguage(tab: Ref<ConsoleTabRecord>) {
  const schemaColumnsStore = useSchemaColumnsStore();
  const connectionsStore = useConnectionsStore();

  const connRecord = computed(() => connectionsStore.connectionRecord(tab.value.connectionId));
  const railColor = computed(() =>
    connRecord.value ? (connRecord.value.color ?? null) : undefined,
  );
  const connectionKind = computed<ConnectionKind | undefined>(() => connRecord.value?.kind);
  const dialect = computed(() => sqlDialectFor(connectionKind.value));

  function splitOptionsFor(d: SqlDialect | undefined) {
    return {
      ...lexOptionsFor(d),
      slashSlashComments: connectionKind.value === 'mongodb',
    };
  }

  const language = computed<EditorLanguageId>(() => consoleLanguageFor(connectionKind.value));

  const ddlQuery = useQuery(() => ({
    ...schemaQueryOptions(tab.value.connectionId ?? ''),
    enabled: !!tab.value.connectionId && dialect.value !== undefined,
  }));
  const documentDdlSchema = computed(() =>
    ddlSchemaFor(tab.value.connectionId ?? '', ddlQuery.data.value, dialect.value),
  );

  const containerPath = computed(() =>
    tab.value.connectionId ? containerPathFor(tab.value.connectionId, tab.value.path) : null,
  );

  watch(
    () =>
      [
        tab.value.connectionId,
        containerPath.value,
        tab.value.connectionId ? schemaColumnsStore.generationFor(tab.value.connectionId) : 0,
      ] as const,
    ([connectionId, path]) => {
      if (connectionId && path) void schemaColumnsStore.ensureSchemaColumns(connectionId, path);
    },
    { immediate: true },
  );

  const ddlSchema = computed(() =>
    schemaColumnsStore.effectiveSchema(
      tab.value.connectionId ?? '',
      containerPath.value ?? '',
      documentDdlSchema.value,
    ),
  );

  const completionSources = computed(() => {
    if (!connectionKind.value || language.value === 'plain') return undefined;
    if (language.value === 'sql') {
      const connectionId = tab.value.connectionId;
      const cached =
        connectionId && containerPath.value
          ? schemaColumnsStore.cachedRelationsFor(connectionId, containerPath.value)
          : [];
      return consoleCompletionSources(
        connectionKind.value,
        connectionId,
        tab.value.path,
        documentDdlSchema.value,
        cached,
      );
    }
    return consoleCompletionSources(connectionKind.value, tab.value.connectionId, tab.value.path);
  });

  const lintSource = computed(() => consoleLintSource(connectionKind.value, ddlSchema.value));
  const hoverSource = computed(() =>
    dialect.value ? sqlHoverSource(dialect.value, ddlSchema.value) : undefined,
  );

  return {
    railColor,
    connectionKind,
    dialect,
    splitOptionsFor,
    language,
    completionSources,
    lintSource,
    hoverSource,
  };
}
