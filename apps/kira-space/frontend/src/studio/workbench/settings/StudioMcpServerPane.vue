<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Field, FieldContent, FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatDateTime } from '@workbench/behaviour/format';
import { computed, useId } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import type { SettingsPaneProps } from '../../../workbench/settings/types';
import { useDbMcpStore } from '../../state/dbmcp';

// Studio MCP settings: server switch, response preview, focus, status and token. Bypasses
// draft/Save: dbMcp.serverEnabled both persists and starts/stops the embedded server in one call.
defineProps<SettingsPaneProps>();

const dbMcpStore = useDbMcpStore();
const settingsStore = useSettingsStore();

// The token's own expiry line: "expired, regenerate" rather than a stale-looking date.
function tokenExpired(expiresAt: string): boolean {
  return !!expiresAt && new Date(expiresAt).getTime() <= Date.now();
}

// Instant-action settings (bypassing draft/Save) report a rejected call here, so the switch that
// snaps back is not the only trace of the failure. One error line serves all actions.
const toggle = useAsyncAction((enabled: boolean) => dbMcpStore.setDbMcpEnabled(enabled));
const preview = useAsyncAction((enabled: boolean) =>
  settingsStore.patchSettings({ dbMcp: { previewResponses: enabled } }),
);
const focus = useAsyncAction((enabled: boolean) =>
  settingsStore.patchSettings({ dbMcp: { focusOnApproval: enabled } }),
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
    regenerate.error.value ??
    install.error.value,
);
function clearActionErrors(): void {
  toggle.error.value = null;
  preview.error.value = null;
  focus.error.value = null;
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

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
const dbMcpEnabledId = useId();
const dbMcpPreviewId = useId();
const dbMcpFocusId = useId();
</script>

<template>
  <div class="contents" v-show="active">
    <!-- This leaf (dbMcp.serverEnabled) both persists and starts/stops the embedded
         Studio MCP server in one call, so it belongs on the action side of the draft/Save line,
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

    <Alert v-if="actionError" variant="error" data-testid="settings-action-error">
      <AlertDescription>{{ actionError }}</AlertDescription>
    </Alert>
  </div>
</template>
