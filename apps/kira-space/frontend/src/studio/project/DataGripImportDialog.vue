<script setup lang="ts">
import type { ConnectionKind } from '@shared/domain/connection';
import type { DataGripPreviewRow, DataGripReportRow } from '@shared/domain/datagrip';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed } from 'vue';
import { useConnectionsStore } from '../state/connections';
import { looksAlreadyImported, useDatagripImportStore } from '../state/datagripImport';
import EngineIcon from '../theme/EngineIcon.vue';

// A real review step. Nothing is written until "Import N connections" is pressed —
// every row here is either checkable (it maps to a supported engine and has enough of a JDBC URL
// to build fields from) or greyed with the specific reason it cannot be. The password itself
// is never fetched or shown here: only an outlook badge, resolved from file reads,
// says what Import is expected to do.

const KIND_ACCENT_CLASS: Record<ConnectionKind, string> = {
  postgres: 'text-conn-cyan',
  mariadb: 'text-conn-blue',
  mysql: 'text-conn-teal',
  sqlite: 'text-conn-violet',
  clickhouse: 'text-conn-orange',
  mongodb: 'text-conn-green',
  redis: 'text-conn-red',
  kafka: 'text-conn-amber',
  sqs: 'text-conn-magenta',
  s3: 'text-conn-olive',
  kibana: 'text-conn-indigo',
};

const OUTLOOK_KEY: Record<string, MessageKey> = {
  'will-attempt': 'studio.project.dataGripImport.outlook.willAttempt.label',
  'from-url': 'studio.project.dataGripImport.outlook.fromUrl.label',
  'not-saved': 'studio.project.dataGripImport.outlook.notSaved.label',
  'store-unsupported': 'studio.project.dataGripImport.outlook.storeUnsupported.label',
};
const OUTLOOK_TONE: Record<string, 'ok' | 'warn' | 'error'> = {
  'will-attempt': 'ok',
  'from-url': 'ok',
  'not-saved': 'warn',
  'store-unsupported': 'error',
};

// Every D11 reason code that can reach ReportRow.Error (a resolveFields skip quoted back with its
// detail joined by ": ", or a bare password-only refusal code) maps to the same label the preview
// step shows, so a report row reads like its preview row.
const REASON_KEY: Record<string, MessageKey> = {
  'unsupported-engine': 'studio.project.dataGripImport.reason.unsupportedEngine.label',
  'unrepresentable-url': 'studio.project.dataGripImport.reason.unrepresentableUrl.label',
  'sqlite-path-not-absolute': 'studio.project.dataGripImport.reason.sqlitePathNotAbsolute.label',
  'no-jdbc-url': 'studio.project.dataGripImport.reason.noUrl.label',
  'password-not-saved': 'studio.project.dataGripImport.outlook.notSaved.label',
  'password-not-found': 'studio.project.dataGripImport.reason.passwordNotFound.label',
  'credential-store-not-found': 'studio.project.dataGripImport.reason.storeNotFound.label',
  'credential-store-unsupported': 'studio.project.dataGripImport.outlook.storeUnsupported.label',
  'credential-store-locked': 'studio.project.dataGripImport.reason.storeLocked.label',
  'secret-storage-unavailable':
    'studio.project.dataGripImport.reason.secretStorageUnavailable.label',
};

const WARNING_KEY: Record<string, MessageKey> = {
  'name-truncated': 'studio.project.dataGripImport.warning.nameTruncated.label',
  'ssh-tunnel-dropped': 'studio.project.dataGripImport.warning.sshTunnelDropped.label',
};

const datagripImportStore = useDatagripImportStore();
const connectionsStore = useConnectionsStore();

const rows = computed(() => datagripImportStore.preview?.rows ?? []);
const checkedCount = computed(() => datagripImportStore.selected.size);
const secretStatus = computed(() => connectionsStore.secretStorage);
const report = computed(() => datagripImportStore.report);
const reportSummary = computed(() => {
  const r = report.value;
  if (!r) return '';
  const created = r.rows.filter((row) => row.created).length;
  const failed = r.rows.length - created;
  const passwords = r.rows.filter((row) => row.passwordImported).length;
  const parts = [
    t('studio.project.dataGripImport.summary.created.body', {
      created,
      count: r.rows.length,
    }),
  ];
  if (passwords > 0)
    parts.push(t('studio.project.dataGripImport.summary.passwords.body', { count: passwords }));
  if (failed > 0) parts.push(t('common.failedCount.label', { count: failed }));
  return parts.join(' ');
});
const reportTone = computed<'info' | 'warn'>(() =>
  (report.value?.rows.some((row) => !row.created) ?? false) ? 'warn' : 'info',
);

function skipLabel(row: DataGripPreviewRow): string {
  const reason = row.skipReason;
  if (reason === 'unsupported-engine')
    return t('studio.project.dataGripImport.skip.unsupportedDriver.label', {
      driver: row.skipDetail ?? '?',
    });
  const key = reason ? REASON_KEY[reason] : undefined;
  if (!key) return row.skipDetail ?? t('studio.project.dataGripImport.skip.notImportable.label');
  const label = t(key);
  return reason === 'sqlite-path-not-absolute'
    ? t('common.withDetail.label', {
        first: label,
        second: row.skipDetail ?? '',
      })
    : label;
}

function warningLabel(code: string): string {
  const key = WARNING_KEY[code];
  return key ? t(key) : code;
}

// row.error is "<code>: <detail>" (a resolveFields skip, D11) or a bare code/message. Only the
// leading segment is a known reason code; an unmapped prefix (a Creator/Validate error) falls
// back to the raw text.
function reportErrorLabel(row: DataGripReportRow): string {
  if (!row.error) return '';
  const sep = row.error.indexOf(': ');
  const code = sep === -1 ? row.error : row.error.slice(0, sep);
  const key = REASON_KEY[code];
  if (!key) return row.error;
  const detail = sep === -1 ? '' : row.error.slice(sep + 2);
  const label = t(key);
  return detail ? t('common.withDetail.label', { first: label, second: detail }) : label;
}

function reportOutcome(row: DataGripReportRow): { label: string; tone: 'ok' | 'warn' | 'error' } {
  if (!row.created)
    return {
      label: t('studio.project.dataGripImport.outcome.notCreated.label', {
        reason: reportErrorLabel(row),
      }),
      tone: 'error',
    };
  if (row.passwordImported)
    return {
      label: t('studio.project.dataGripImport.outcome.createdWithPassword.label'),
      tone: 'ok',
    };
  if (row.error)
    return {
      label: t('studio.project.dataGripImport.outcome.createdWithNote.label', {
        reason: reportErrorLabel(row),
      }),
      tone: 'warn',
    };
  return { label: t('common.created.label'), tone: 'ok' };
}

// No auto-close after confirm: the store's `report` flips the template into the results view and
// the user closes explicitly.
const pick = useAsyncAction(() => datagripImportStore.pickAndScanDataGripProject());
const confirm = useAsyncAction(() => datagripImportStore.confirmDataGripImport());
async function onConfirm(): Promise<void> {
  await confirm.run();
}
</script>

<template>
  <Dialog
    v-if="datagripImportStore.open"
    :open="true"
    @update:open="(v) => !v && datagripImportStore.closeDataGripImportDialog()"
  >
    <DialogContent size="xl" fixed-height data-testid="datagrip-import-dialog">
      <DialogHeader close-testid="datagrip-import-dialog-close">
        <DialogTitle>{{
          report
            ? t('studio.project.dataGripImport.report.title')
            : t('studio.project.dataGripImport.dialog.title')
        }}</DialogTitle>
      </DialogHeader>

      <DialogBody flush>
        <div v-if="!report" class="flex flex-col gap-2 p-3">
          <Alert
            v-if="secretStatus && !secretStatus.available"
            data-testid="datagrip-secrets-unavailable"
            variant="warn"
          >
            <AlertDescription>
              {{
                secretStatus.reason ?? t('studio.project.dataGripImport.secretsUnavailable.body')
              }}
            </AlertDescription>
          </Alert>
          <Alert
            v-if="datagripImportStore.error"
            variant="error"
            data-testid="datagrip-import-error"
          >
            <AlertDescription>{{ datagripImportStore.error }}</AlertDescription>
          </Alert>

          <div
            v-if="datagripImportStore.preview"
            class="flex flex-col py-1"
            data-testid="datagrip-preview-rows"
          >
            <div
              v-for="row in rows"
              :key="row.uuid"
              class="flex items-center gap-1.5 h-6.5 px-3 text-kira-md"
              :class="{ 'text-muted-foreground': !row.importable }"
              :data-testid="`datagrip-row-${row.uuid}`"
              :data-importable="row.importable"
            >
              <span class="w-4 shrink-0 flex justify-center">
                <Checkbox
                  v-if="row.importable"
                  :model-value="datagripImportStore.selected.has(row.uuid)"
                  class="size-3.5"
                  @update:model-value="datagripImportStore.toggleDataGripRow(row.uuid)"
                >
                  <CodiconIcon name="check" :size="12" />
                </Checkbox>
              </span>
              <span
                v-if="row.importable"
                class="shrink-0 flex"
                :class="KIND_ACCENT_CLASS[row.kind as ConnectionKind]"
              >
                <EngineIcon :kind="row.kind as ConnectionKind" :size="15" />
              </span>
              <span v-else class="shrink-0 flex text-subtle"
                ><CodiconIcon name="circle-slash" :size="13" /></span
              >

              <span class="min-w-0 max-w-56 truncate">{{ row.name }}</span>

              <template v-if="row.importable">
                <span class="whitespace-nowrap" :class="rowMeta">
                  {{ row.host ? `${row.host}:${row.port}` : ''
                  }}<span v-if="row.database">/{{ row.database }}</span>
                </span>
                <span v-if="row.username" class="whitespace-nowrap" :class="rowMeta">{{
                  row.username
                }}</span>
                <Tooltip v-if="looksAlreadyImported(row)">
                  <TooltipTrigger as-child>
                    <Badge variant="warn" data-testid="datagrip-row-duplicate">
                      {{ t('studio.project.dataGripImport.badge.duplicate.label') }}
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent>{{
                    t('studio.project.dataGripImport.badge.duplicate.tooltip')
                  }}</TooltipContent>
                </Tooltip>
                <Badge
                  v-if="row.passwordOutlook"
                  class="ml-auto"
                  :variant="OUTLOOK_TONE[row.passwordOutlook]"
                  data-testid="datagrip-row-outlook"
                >
                  {{
                    t(
                      OUTLOOK_KEY[row.passwordOutlook] ??
                        'studio.project.dataGripImport.outlook.notSaved.label',
                    )
                  }}
                </Badge>
              </template>
              <template v-else>
                <span class="ml-auto" :class="rowMeta" data-testid="datagrip-row-skip-reason">{{
                  skipLabel(row)
                }}</span>
              </template>

              <span v-if="row.warnings.length > 0" class="flex shrink-0 gap-0.5">
                <Tooltip v-for="w in row.warnings" :key="w">
                  <TooltipTrigger as-child>
                    <span class="size-4 flex items-center justify-center shrink-0 text-subtle">
                      <CodiconIcon name="warning" :size="12" />
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>{{ warningLabel(w) }}</TooltipContent>
                </Tooltip>
              </span>
            </div>
            <InlineState v-if="rows.length === 0" kind="empty">{{
              t('studio.project.dataGripImport.rows.empty')
            }}</InlineState>
          </div>
        </div>

        <!-- Review finding: the report Import returns used to be discarded on close with no display
         at all — every per-row outcome (created, password imported, or the specific reason it
         wasn't) is now shown here instead of auto-closing. -->
        <div v-else class="flex flex-col gap-2 p-3">
          <Alert data-testid="datagrip-report-summary" :variant="reportTone">
            <AlertDescription>
              {{ reportSummary }}
            </AlertDescription>
          </Alert>

          <div class="flex flex-col py-1" data-testid="datagrip-report-rows">
            <div
              v-for="row in report.rows"
              :key="row.uuid"
              class="flex items-center gap-1.5 h-6.5 px-3 text-kira-md"
              :data-testid="`datagrip-report-row-${row.uuid}`"
            >
              <span class="shrink-0 flex" :class="row.created ? 'text-ok' : 'text-error'">
                <CodiconIcon :name="row.created ? 'check' : 'error'" :size="13" />
              </span>
              <span class="min-w-0 max-w-56 truncate">{{ row.name }}</span>
              <Badge
                class="ml-auto"
                :variant="reportOutcome(row).tone"
                data-testid="datagrip-report-row-outcome"
              >
                {{ reportOutcome(row).label }}
              </Badge>
            </div>
          </div>
        </div>
      </DialogBody>

      <DialogFooter>
        <template v-if="!report" #start>
          <FieldDescription class="font-data">{{
            datagripImportStore.projectPath
          }}</FieldDescription>
        </template>
        <template v-if="!report">
          <Button
            v-if="!datagripImportStore.preview"
            variant="dialog"
            size="kira-lg"
            data-testid="datagrip-import-choose-another"
            :pending="pick.pending.value"
            @click="pick.run"
          >
            {{ t('studio.project.dataGripImport.chooseAnother.label') }}
          </Button>
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="datagrip-import-cancel"
            @click="datagripImportStore.closeDataGripImportDialog"
          >
            {{ t('common.action.cancel.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="datagrip-import-confirm"
            :disabled="checkedCount === 0"
            :pending="confirm.pending.value"
            @click="onConfirm"
          >
            {{ t('studio.project.dataGripImport.import.label', { count: checkedCount }) }}
          </Button>
        </template>
        <Button
          v-else
          variant="dialog-primary"
          size="kira-lg"
          data-testid="datagrip-import-report-close"
          @click="datagripImportStore.closeDataGripImportDialog"
        >
          {{ t('common.action.close.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
