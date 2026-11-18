<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
import DetailSection from '@theme/components/DetailSection.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import { copyAndAnnounce } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, watch } from 'vue';
import { control } from '../../bridge/control';
import { useCliTools, useRefreshCliTools } from '../../state/cliTools';
import type { CliTool } from '../../state/cliToolsDomain';
import type { SettingsPaneProps } from './types';

// Setup guide for the local CLIs the app shells out to (a code host's and an issue tracker's). The
// CLI owns credentials; this pane only shows what the app sees and the commands to fix it.
const props = defineProps<SettingsPaneProps>();

const tools = useCliTools(() => props.active);
const refresh = useRefreshCliTools();
watch(
  () => props.active,
  (active) => {
    if (active && tools.data.value) void tools.refetch();
  },
);

type Tone = 'ok' | 'warn' | 'error';
const STATE: Record<string, { tone: Tone; label: MessageKey }> = {
  ok: { tone: 'ok', label: 'space.settings.cliTools.state.ok.status' },
  unauthenticated: { tone: 'warn', label: 'space.settings.cliTools.state.unauthenticated.status' },
  notFound: { tone: 'error', label: 'space.settings.cliTools.state.notFound.status' },
};
const STEP_LABEL: Record<string, MessageKey> = {
  install: 'space.settings.cliTools.step.install.label',
  extension: 'space.settings.cliTools.step.extension.label',
  defaults: 'space.settings.cliTools.step.defaults.label',
  login: 'space.settings.cliTools.step.login.label',
  loginToken: 'space.settings.cliTools.step.loginToken.label',
  check: 'space.settings.cliTools.step.check.label',
};

const list = computed(() => tools.data.value?.tools ?? []);
const state = (tool: CliTool) => STATE[tool.status.kind] ?? STATE.notFound;
const loadError = computed(() => tools.error.value ?? refresh.error.value);

function details(tool: CliTool): string[] {
  const s = tool.status;
  return [s.version, s.account, s.site].filter((v) => v !== '');
}
function openDocs(url: string): void {
  fireAndForget(control.openExternal(url), 'settings.cliTools');
}
function copy(command: string): void {
  fireAndForget(copyAndAnnounce(command), 'settings.cliTools');
}
</script>

<template>
  <div class="contents" v-show="active">
    <FieldDescription>{{ t('space.settings.cliTools.description') }}</FieldDescription>

    <FieldError v-if="loadError" data-testid="cli-tools-error">{{
      errorText(loadError)
    }}</FieldError>

    <DetailSection
      v-for="tool in list"
      :key="tool.provider"
      :title="tool.label"
      :data-testid="`cli-tool-${tool.provider}`"
    >
      <template #actions>
        <Badge :variant="state(tool).tone" :data-testid="`cli-tool-status-${tool.provider}`">
          {{ t(state(tool).label) }}
        </Badge>
      </template>
      <span class="text-kira-sm text-muted-foreground">{{ tool.cli }}</span>

      <div
        v-if="details(tool).length > 0"
        class="flex flex-wrap gap-x-3 text-kira-sm text-muted-foreground"
        :data-testid="`cli-tool-details-${tool.provider}`"
      >
        <span v-for="d in details(tool)" :key="d" class="font-data">{{ d }}</span>
      </div>
      <Alert
        v-if="tool.status.kind !== 'ok' && tool.status.reason"
        :variant="state(tool).tone === 'error' ? 'warn' : 'info'"
        :data-testid="`cli-tool-reason-${tool.provider}`"
      >
        {{ tool.status.reason }}
      </Alert>

      <ol class="flex flex-col gap-1.5" :data-testid="`cli-tool-steps-${tool.provider}`">
        <li v-for="(step, i) in tool.setup.steps" :key="i" class="flex flex-col gap-0.5">
          <span class="text-kira-sm text-muted-foreground">
            {{ STEP_LABEL[step.purpose] ? t(STEP_LABEL[step.purpose]) : step.purpose }}
          </span>
          <span class="flex items-center gap-1">
            <CodeBlock class="min-w-0 flex-1">{{ step.command }}</CodeBlock>
            <TooltipIconButton
              icon="copy"
              :label="t('common.copyCommand.label')"
              :data-testid="`cli-tool-copy-${tool.provider}-${step.purpose}`"
              @click="copy(step.command)"
            />
          </span>
        </li>
      </ol>

      <div>
        <Button
          variant="link"
          size="inline"
          :data-testid="`cli-tool-docs-${tool.provider}`"
          @click="openDocs(tool.setup.docsUrl)"
        >
          {{ t('space.settings.cliTools.docs.label') }}
        </Button>
      </div>
    </DetailSection>

    <div>
      <Button
        variant="dialog"
        size="kira-lg"
        :pending="refresh.isPending.value"
        data-testid="cli-tools-refresh"
        @click="refresh.mutate()"
      >
        {{ t('space.settings.cliTools.refresh.label') }}
      </Button>
    </div>
  </div>
</template>
