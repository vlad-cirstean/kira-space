<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { paneFrame } from '@theme/lib/frames';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import MonacoHost from '../editor/MonacoHost.vue';
import { useConnectionsStore } from '../state/connections';
import {
  ddlParseSummary,
  ensureDdl,
  schemaDialectFor,
  sqlKeywordCompletionSourceFor,
  useSaveDdlMutation,
  useSchemaDialogStore,
} from '../state/schemas';

// The DDL document a user pastes for one connection, staged until Save — the
// draft is component-local, cloned at open; Cancel, Escape, the ✕ and the backdrop all discard
// silently (DialogFrame's own @close); only Save writes. Reuses the landed staging shape
// exactly rather than inventing a second one. schemaDialectFor/ddlParseSummary (state/schemas.ts)
// are this file's one dispatch point into the SQL surface — SPEC forbids project/ importing
// views/ directly, the same rule state/viewCommands.ts exists to satisfy for other callers.
//
// The "Fill from connection" button is gone — the schema metadata it staged at N
// round trips (one treeDefinition call per relation) is now served automatically, with no manual
// step, by the console/data-tab's own schema-aware completion (state/schemaColumns.ts). This
// document is now a deliberate override for what that cache cannot serve: a schema that doesn't
// exist yet, an engine with no SchemaColumns capability, or a connection this app cannot
// introspect. The dialog, saveDdl/ensureDdl and the connection-row menu entry are unchanged.

const DDL_SOURCES = {
  pgDump: 'pg_dump --schema-only',
  showCreate: 'SHOW CREATE TABLE',
  sqliteSchema: '.schema',
};

const connectionsStore = useConnectionsStore();
const schemaDialogStore = useSchemaDialogStore();
const { mutateAsync: saveDdl, isPending: saving } = useSaveDdlMutation();
const draft = ref('');
const loaded = ref('');
const dirty = computed(() => draft.value !== loaded.value);

// `draft` (the editor's own live doc) updates every keystroke, but the
// parse summary below reads this instead — a full Lezer parse of the whole document (measured up
// to ~58ms on a 200-table schema, against the app's own 50ms interaction budget) has no business
// running on every keystroke. TIMING.preview, the same wait as MonacoHost.vue's marker debounce.
// An external load (the watcher
// below) writes both refs immediately, with no delay — only typing goes through the timer.
const debouncedDraft = ref('');
const setDebouncedDraft = useDebounceFn((text: string) => {
  debouncedDraft.value = text;
}, TIMING.preview);
onBeforeUnmount(() => setDebouncedDraft.cancel());

const connectionId = computed(() => schemaDialogStore.connectionId);
const connectionKind = computed(() => connectionsStore.connectionRecord(connectionId.value)?.kind);
const connectionName = computed(
  () => connectionsStore.connectionRecord(connectionId.value)?.name ?? '',
);
const dialect = computed(() => schemaDialectFor(connectionKind.value));
// P60b §6.2: MonacoHost has no equivalent of CodeMirror's own implicit language-data keyword
// source — `:autocomplete="true"` alone offered dialect-correct keyword/type completion before
// (lang-sql's own override-less default); this is that source, made explicit.
const completionSources = computed(() => {
  const source = sqlKeywordCompletionSourceFor(connectionKind.value);
  return source && [source];
});

// The reset must happen synchronously, before the `await` below, and the
// response must be discarded if the dialog has since moved on to a different connection — without
// both, a fast open of connection B landing before a slow open of connection A's ensureDdl
// resolves would let A's stale DDL text land in B's (still fully editable) draft, and Save would
// write it into B's connection_ddl row.
watch(
  () => schemaDialogStore.connectionId,
  async (id) => {
    // The dialog only unmounts on close (ProjectPanel.vue's `v-if`
    // gates on `open`, not `connectionId`) — switching connections while it stays open reuses
    // this same instance, so a debounce timer left running from typing in the previous
    // connection's DDL text would otherwise fire later and clobber this one's debouncedDraft.
    setDebouncedDraft.cancel();
    draft.value = '';
    loaded.value = '';
    debouncedDraft.value = '';
    if (!id) return;
    const ddl = await ensureDdl(id);
    if (schemaDialogStore.connectionId !== id) return; // superseded by a later open
    draft.value = ddl;
    loaded.value = ddl;
    debouncedDraft.value = ddl;
  },
  { immediate: true },
);

// A live parse summary — "N tables, M columns" — the only feedback that tells a user their DDL is
// being understood. ddl.ts's own extractor never throws and never reports a structured parse error
// (an unrecognised statement is silently skipped, not a failure), so "recognised nothing at all"
// is the one failure state this can show.
const parseSummary = computed(() => ddlParseSummary(connectionKind.value, debouncedDraft.value));

function onDocChange(text: string): void {
  draft.value = text;
  void setDebouncedDraft(text);
}

// SettingsDialog.vue's own pattern (a saveError ref plus a footer strip) mirrored exactly — this
// file's own header comment already claimed to reuse the staging shape, but had no catch at all: a
// rejected schemaSet became an unhandled promise rejection from a template @click, the dialog
// stayed open with nothing shown, and Save looked merely slow rather than failed.
const saveError = ref<string | null>(null);

async function onSave(): Promise<void> {
  const id = connectionId.value;
  if (!id || saving.value) return;
  saveError.value = null;
  try {
    await saveDdl({ connectionId: id, ddl: draft.value });
    schemaDialogStore.closeSchemaDialog();
  } catch (err) {
    saveError.value = errorText(err);
  }
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && schemaDialogStore.closeSchemaDialog()">
    <DialogContent size="xl" fixed-height :dirty="dirty" data-testid="schema-dialog">
      <DialogHeader close-testid="schema-dialog-close">
        <DialogTitle>{{
          connectionName
            ? t('studio.project.schemaDialog.dialog.named.title', { connectionName })
            : t('studio.project.schemaDialog.dialog.title')
        }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <FieldDescription>{{ t('studio.project.schemaDialog.intro.body') }}</FieldDescription>
        <div :class="[paneFrame, 'flex-1']">
          <!-- `completionSources` (state/schemas.ts's own sqlKeywordCompletionSourceFor
             dispatch, project/ may not import views/ per biome.json) gives dialect-correct
             keyword/type-name completion here — useful for hand-typing VARCHAR, NUMERIC(10,2),
             REFERENCES, NOT NULL. Relation/column completion from what's being typed in THIS
             document is a separate, larger piece of work and is left for a later phase. -->
          <MonacoHost
            :doc="draft"
            language="sql"
            :sql-dialect="dialect"
            :autocomplete="true"
            :completion-sources="completionSources"
            @update:doc="onDocChange"
          />
        </div>
        <Alert variant="info" data-testid="schema-parse-summary">
          <AlertDescription>
            <span v-if="parseSummary">{{ parseSummary }}</span>
            <FieldDescription v-else>
              <CatalogText k="studio.project.schemaDialog.pasteHint.hint">
                <template #pgDump><span class="font-data">{{ DDL_SOURCES.pgDump }}</span></template>
                <template #showCreate
                  ><span class="font-data">{{ DDL_SOURCES.showCreate }}</span></template
                >
                <template #sqliteSchema
                  ><span class="font-data">{{ DDL_SOURCES.sqliteSchema }}</span></template
                >
              </CatalogText>
            </FieldDescription>
          </AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <template #start>
          <FieldError v-if="saveError" data-testid="schema-save-error">{{ saveError }}</FieldError>
          <FieldDescription v-else
            ><CatalogText k="studio.project.connectionScope.hint">
              <template #connectionName
                ><span class="font-data">{{ connectionName }}</span></template
              >
            </CatalogText></FieldDescription
          >
        </template>
        <Button variant="dialog" size="kira-lg" @click="schemaDialogStore.closeSchemaDialog">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button variant="dialog-primary" size="kira-lg" :pending="saving" @click="onSave">
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
