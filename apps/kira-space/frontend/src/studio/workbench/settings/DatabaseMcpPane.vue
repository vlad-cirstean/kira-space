<script setup lang="ts">
import type { ConnectionSummary } from '@shared/domain/connection';
import { useQuery } from '@tanstack/vue-query';
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Field, FieldContent, FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatDateTime } from '@workbench/behaviour/format';
import { computed, ref, useId } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import type { SettingsPaneProps } from '../../../workbench/settings/types';
import { useApiAgentAccess } from '../../api/state/apiConfig';
import ClusterMcpDialog from '../../kubernetes/components/ClusterMcpDialog.vue';
import { useClusters } from '../../kubernetes/queries';
import { useConnectionDialogStore } from '../../state/connectionDialogs';
import { useConnectionsStore } from '../../state/connections';
import { useDbMcpStore } from '../../state/dbmcp';
import { loadMaskRuleCounts, maskRuleCountsQueryKey } from '../../state/maskRules';
import ScriptsMcpSection from './ScriptsMcpSection.vue';

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Database MCP'"` branch. Bypasses draft/Save entirely —
// dbMcp.serverEnabled both persists and starts/stops the embedded DB MCP server in one call.
defineProps<SettingsPaneProps>();

const connectionsStore = useConnectionsStore();
const connectionDialogStore = useConnectionDialogStore();
const dbMcpStore = useDbMcpStore();
const settingsStore = useSettingsStore();
const maskRuleCountsQuery = useQuery({
  queryKey: maskRuleCountsQueryKey,
  queryFn: loadMaskRuleCounts,
});

// M1 §6.2: the DB MCP token's own expiry line — "expired, regenerate" rather than a stale-looking
// date once the instant has passed.
function tokenExpired(expiresAt: string): boolean {
  return !!expiresAt && new Date(expiresAt).getTime() <= Date.now();
}

// Instant-action settings (bypassing draft/Save) report a rejected call here, so the switch that
// snaps back is not the only trace of the failure. One error line serves all three actions.
const toggle = useAsyncAction((enabled: boolean) => dbMcpStore.setDbMcpEnabled(enabled));
const preview = useAsyncAction((enabled: boolean) =>
  settingsStore.patchSettings({ dbMcp: { previewResponses: enabled } }),
);
const focus = useAsyncAction((enabled: boolean) =>
  settingsStore.patchSettings({ dbMcp: { focusOnApproval: enabled } }),
);
const apiRequests = useAsyncAction((enabled: boolean) =>
  settingsStore.patchSettings({ dbMcp: { apiRequests: enabled } }),
);
const apiApproval = useAsyncAction((mode: 'always' | 'never') =>
  settingsStore.patchSettings({ dbMcp: { apiApproval: mode } }),
);
const regenerate = useAsyncAction(() => dbMcpStore.regenerateDbMcpToken());
const install = useAsyncAction(() => dbMcpStore.installDbMcpAgent());
const dbMcpToggling = toggle.pending;
const dbMcpRegenerating = regenerate.pending;
const dbMcpInstalling = install.pending;
const actionError = computed(
  () =>
    toggle.error.value ??
    preview.error.value ??
    focus.error.value ??
    apiRequests.error.value ??
    apiApproval.error.value ??
    regenerate.error.value ??
    install.error.value,
);
function clearActionErrors(): void {
  toggle.error.value = null;
  preview.error.value = null;
  focus.error.value = null;
  apiRequests.error.value = null;
  apiApproval.error.value = null;
  regenerate.error.value = null;
  install.error.value = null;
}
async function onToggleDbMcpEnabled(enabled: boolean): Promise<void> {
  clearActionErrors();
  await toggle.run(enabled);
}
async function onTogglePreviewResponses(enabled: boolean): Promise<void> {
  clearActionErrors();
  await preview.run(enabled);
}
async function onToggleFocusOnApproval(enabled: boolean): Promise<void> {
  clearActionErrors();
  await focus.run(enabled);
}
async function onToggleApiRequests(enabled: boolean): Promise<void> {
  clearActionErrors();
  await apiRequests.run(enabled);
}
async function onApiApprovalChange(value: unknown): Promise<void> {
  clearActionErrors();
  await apiApproval.run(value === 'never' ? 'never' : 'always');
}
const agentAccess = useApiAgentAccess();
const sharedVariableCount = computed(() =>
  (agentAccess.data.value ?? []).reduce((n, env) => n + env.variables.length, 0),
);
async function onRegenerateDbMcpToken(): Promise<void> {
  clearActionErrors();
  await regenerate.run();
}
async function onInstallDbMcpAgent(): Promise<void> {
  clearActionErrors();
  await install.run();
}

const dbMcpInstallMessage = computed(() => {
  const result = dbMcpStore.installResult;
  if (!result) return null;
  switch (result.outcome) {
    case 'installed':
      return t('common.agentRegistered.description');
    case 'notFound':
      return t('common.agentCliNotFound.error');
    case 'installFailed':
      return t('common.agentRegistrationRefused.error', { detail: result.detail });
    default:
      return null;
  }
});

const dbMcpTokenExpired = computed(() => tokenExpired(dbMcpStore.status.expiresAt));

// Exposure and the three permission modes live in the connection's file; this list is a glance plus
// the way into the file.
// M2 §7.3: the row's own description glance — first line only, "" when unset.
/** Status line of a connection's result review; empty while every mode follows the app setting. */
function reviewStatus(conn: ConnectionSummary): string {
  const { read, write, ddl } = conn.mcpReview;
  if (read === 'inherit' && write === 'inherit' && ddl === 'inherit') return '';
  if (read === 'review' && write === 'review' && ddl === 'review')
    return t('studio.settings.dbMcp.reviewAlways.status');
  if (read === 'send' && write === 'send' && ddl === 'send')
    return t('studio.settings.dbMcp.reviewNever.status');
  return t('studio.settings.dbMcp.reviewMixed.status');
}

function mcpDescriptionFirstLine(conn: ConnectionSummary): string {
  return conn.mcpDescription.split('\n', 1)[0] ?? '';
}

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
// P440: Kubernetes clusters get the same read-only glance; the dialog edits modes and description.
const kubeClusters = useClusters();
const kubeList = computed(() => kubeClusters.data.value?.clusters ?? []);
const kubeEditId = ref('');
const kubeEditOpen = computed({
  get: () => kubeEditId.value !== '',
  set: (v) => {
    if (!v) kubeEditId.value = '';
  },
});

const dbMcpEnabledId = useId();
const dbMcpPreviewId = useId();
const dbMcpFocusId = useId();
const dbMcpApiId = useId();
const dbMcpApiApprovalId = useId();
</script>

<template>
  <div class="contents" v-show="active">
    <!-- This leaf (dbMcp.serverEnabled) both persists and starts/stops the embedded
         DB MCP server in one call, so it belongs on the action side of the draft/Save line,
         never mixed with it. Toggle, then command, then button, strictly in that DOM order
         ("enabling is never a silent action"). -->
    <Field orientation="horizontal" class="items-start">
      <Switch
        :id="dbMcpEnabledId"
        :model-value="settingsStore.dbMcp.serverEnabled"
        :disabled="dbMcpToggling"
        data-testid="settings-db-mcp-enabled"
        @update:model-value="(v) => onToggleDbMcpEnabled(v === true)"
      />
      <FieldContent>
        <Label :for="dbMcpEnabledId">{{ t('studio.settings.dbMcp.enable.label') }}</Label>
        <FieldDescription>{{ t('studio.settings.dbMcp.enable.description') }}</FieldDescription>
        <FieldDescription data-testid="dbmcp-sessions-hint">{{
          t('studio.settings.dbMcp.sessions.hint')
        }}</FieldDescription>
      </FieldContent>
    </Field>

    <!-- Meaningful before the server starts, so always visible. Instant like the toggle above. -->
    <Field orientation="horizontal" class="items-start">
      <Switch
        :id="dbMcpPreviewId"
        :model-value="settingsStore.dbMcp.previewResponses"
        :disabled="preview.pending.value"
        data-testid="settings-db-mcp-preview"
        @update:model-value="(v) => onTogglePreviewResponses(v === true)"
      />
      <FieldContent>
        <Label :for="dbMcpPreviewId">{{ t('studio.settings.dbMcp.preview.label') }}</Label>
        <FieldDescription>{{ t('studio.settings.dbMcp.preview.description') }}</FieldDescription>
      </FieldContent>
    </Field>

    <Field orientation="horizontal" class="items-start">
      <Switch
        :id="dbMcpFocusId"
        :model-value="settingsStore.dbMcp.focusOnApproval"
        :disabled="focus.pending.value"
        data-testid="settings-db-mcp-focus"
        @update:model-value="(v) => onToggleFocusOnApproval(v === true)"
      />
      <FieldContent>
        <Label :for="dbMcpFocusId">{{ t('studio.settings.dbMcp.focus.label') }}</Label>
        <FieldDescription>{{ t('studio.settings.dbMcp.focus.description') }}</FieldDescription>
      </FieldContent>
    </Field>

    <Field orientation="horizontal" class="items-start">
      <Switch
        :id="dbMcpApiId"
        :model-value="settingsStore.dbMcp.apiRequests"
        :disabled="apiRequests.pending.value"
        data-testid="settings-db-mcp-api"
        @update:model-value="(v) => onToggleApiRequests(v === true)"
      />
      <FieldContent>
        <Label :for="dbMcpApiId">{{ t('studio.settings.dbMcp.api.label') }}</Label>
        <FieldDescription>{{ t('studio.settings.dbMcp.api.description') }}</FieldDescription>
        <FieldDescription data-testid="settings-db-mcp-api-count">{{
          t('studio.settings.dbMcp.api.count.status', {
            count: sharedVariableCount,
            environments: (agentAccess.data.value ?? []).length,
          })
        }}</FieldDescription>
      </FieldContent>
    </Field>
    <Field v-if="settingsStore.dbMcp.apiRequests">
      <Label :for="dbMcpApiApprovalId">{{ t('studio.settings.dbMcp.apiApproval.label') }}</Label>
      <NativeSelect
        :id="dbMcpApiApprovalId"
        variant="bordered"
        size="kira-lg"
        class="self-start"
        data-testid="settings-db-mcp-api-approval"
        :model-value="settingsStore.dbMcp.apiApproval"
        :disabled="apiApproval.pending.value"
        @update:model-value="onApiApprovalChange"
      >
        <option value="always">{{ t('studio.settings.dbMcp.apiApproval.always.option') }}</option>
        <option value="never">{{ t('studio.settings.dbMcp.apiApproval.never.option') }}</option>
      </NativeSelect>
      <Alert
        v-if="settingsStore.dbMcp.apiApproval === 'never'"
        variant="warn"
        data-testid="settings-db-mcp-api-never-warning"
      >
        <AlertDescription>{{ t('studio.settings.dbMcp.apiApproval.never.body') }}</AlertDescription>
      </Alert>
    </Field>

    <template v-if="settingsStore.dbMcp.serverEnabled">
      <Alert v-if="dbMcpStore.status.error" variant="error" data-testid="db-mcp-error">
        <AlertDescription>{{ dbMcpStore.status.error }}</AlertDescription>
      </Alert>
      <!-- Regenerate must render whenever the server is running, in both branches below —
           the expired-token message further down ("regenerate it above") has no remedy otherwise
           once a command was shown (the common case: the command shows across ordinary
           restarts now, so this is no longer the rare branch it used to be). -->
      <template v-else-if="dbMcpStore.status.running">
        <template v-if="dbMcpStore.status.command">
          <CodeBlock class="select-all" data-testid="db-mcp-command">{{
            dbMcpStore.status.command
          }}</CodeBlock>
          <Button
            variant="dialog"
            size="kira-lg"
            class="self-start"
            :pending="dbMcpInstalling"
            data-testid="db-mcp-install-button"
            @click="onInstallDbMcpAgent"
            >{{
              dbMcpStore.status.agentAvailable
                ? t('common.registerWithAgent.label')
                : t('studio.settings.dbMcp.copyCommandAbove.label')
            }}
          </Button>
          <FieldDescription v-if="dbMcpInstallMessage" data-testid="db-mcp-install-outcome">
            {{ dbMcpInstallMessage }}
          </FieldDescription>
        </template>
        <FieldDescription v-else data-testid="db-mcp-no-token">
          {{ t('studio.settings.dbMcp.noToken.hint') }}
        </FieldDescription>
        <Button
          variant="dialog"
          size="kira-lg"
          class="self-start"
          :pending="dbMcpRegenerating"
          data-testid="db-mcp-regenerate-button"
          @click="onRegenerateDbMcpToken"
        >
          {{ t('studio.settings.dbMcp.regenerate.label') }}
        </Button>
      </template>
      <FieldDescription
        v-if="dbMcpStore.status.running && dbMcpStore.status.expiresAt"
        data-testid="db-mcp-token-expiry"
      >
        {{
          dbMcpTokenExpired
            ? t('studio.settings.dbMcp.tokenExpired.body')
            : t('studio.settings.dbMcp.tokenValid.body', {
                date: formatDateTime(dbMcpStore.status.expiresAt),
              })
        }}
      </FieldDescription>
    </template>

    <FieldLegend>{{ t('studio.settings.dbMcp.exposed.label') }}</FieldLegend>
    <FieldDescription>{{ t('studio.settings.dbMcp.exposed.description') }}</FieldDescription>
    <FieldDescription>{{ t('studio.settings.dbMcp.defaults.hint') }}</FieldDescription>
    <ul
      v-if="connectionsStore.records.length"
      class="flex flex-col m-0 p-0 list-none gap-0.5"
      data-testid="db-mcp-connections-list"
    >
      <li v-for="conn in connectionsStore.records" :key="conn.id">
        <Card
          variant="list"
          class="flex-row items-center justify-between gap-1.5"
          :data-testid="`db-mcp-connection-row-${conn.id}`"
        >
          <div class="flex flex-col min-w-0 gap-0.5">
            <span class="break-words text-fg text-kira-md">{{ conn.name }}</span>
            <FieldDescription>{{
              t(
                conn.mcpAutoExplain
                  ? 'studio.settings.dbMcp.modesPlans.status'
                  : 'studio.settings.dbMcp.modes.status',
                {
                  read: conn.mcpReadMode,
                  write: conn.mcpWriteMode,
                  ddl: conn.mcpDdlMode,
                },
              )
            }}</FieldDescription>
            <FieldDescription
              v-if="reviewStatus(conn)"
              :data-testid="`db-mcp-connection-review-${conn.id}`"
              >{{
                reviewStatus(conn)
              }}</FieldDescription
            >
            <FieldDescription v-if="mcpDescriptionFirstLine(conn)">{{
              mcpDescriptionFirstLine(conn)
            }}</FieldDescription>
            <!-- M5 §7.5: extends this existing read-only glance — no second full editor
               here (M2's own established split); editing lives in the connection's own
               Privacy tab. -->
            <FieldDescription
              v-if="maskRuleCountsQuery.data.value?.[conn.id]"
              :data-testid="`db-mcp-connection-masked-${conn.id}`"
              >{{
                t('studio.settings.dbMcp.masked.status', {
                  count: maskRuleCountsQuery.data.value?.[conn.id] ?? 0,
                })
              }}</FieldDescription
            >
          </div>
          <div class="flex shrink-0 items-center gap-1.5">
            <Badge
              :variant="conn.mcpEnabled ? 'ok' : 'default'"
              :data-testid="`db-mcp-connection-${conn.id}`"
              >{{
                conn.mcpEnabled ? t('studio.settings.dbMcp.exposed.status') : t('common.off.status')
              }}</Badge
            >
            <Button
              variant="dialog"
              size="kira-lg"
              :disabled="!conn.file"
              :data-testid="`db-mcp-connection-edit-${conn.id}`"
              @click="connectionDialogStore.openFile(conn.file)"
            >
              {{ t('common.action.edit.label') }}
            </Button>
          </div>
        </Card>
      </li>
    </ul>
    <FieldDescription v-else data-testid="db-mcp-connections-empty">
      {{ t('studio.settings.dbMcp.empty.empty') }}
    </FieldDescription>

    <FieldLegend>{{ t('studio.settings.dbMcp.kube.legend.label') }}</FieldLegend>
    <FieldDescription>{{ t('studio.settings.dbMcp.kube.intro.description') }}</FieldDescription>
    <ul
      v-if="kubeList.length"
      class="flex flex-col m-0 p-0 list-none gap-0.5"
      data-testid="db-mcp-kube-list"
    >
      <li v-for="cluster in kubeList" :key="cluster.id">
        <Card
          variant="list"
          class="flex-row items-center justify-between gap-1.5"
          :data-testid="`db-mcp-kube-row-${cluster.id}`"
        >
          <div class="flex flex-col min-w-0 gap-0.5">
            <span class="break-words text-fg text-kira-md">{{ cluster.context }}</span>
            <FieldDescription>{{
              t('studio.settings.dbMcp.kube.modes.status', {
                discover: cluster.mcp.discover,
                list: cluster.mcp.list,
                describe: cluster.mcp.describe,
                logs: cluster.mcp.logs,
              })
            }}</FieldDescription>
            <FieldDescription v-if="cluster.mcp.description">{{
              cluster.mcp.description.split('\n', 1)[0]
            }}</FieldDescription>
          </div>
          <Button
            variant="dialog"
            size="kira-lg"
            :data-testid="`db-mcp-kube-edit-${cluster.id}`"
            @click="kubeEditId = cluster.id"
          >
            {{ t('common.action.edit.label') }}
          </Button>
        </Card>
      </li>
    </ul>
    <FieldDescription v-else data-testid="db-mcp-kube-empty">
      {{ t('studio.settings.dbMcp.kube.empty.empty') }}
    </FieldDescription>
    <ClusterMcpDialog v-if="kubeEditId" v-model:open="kubeEditOpen" :cluster-id="kubeEditId" />

    <ScriptsMcpSection />

    <Alert v-if="actionError" variant="error" data-testid="settings-action-error">
      <AlertDescription>{{ actionError }}</AlertDescription>
    </Alert>
  </div>
</template>
