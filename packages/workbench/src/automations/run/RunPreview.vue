<script setup lang="ts">
import type { ScriptRunPreview } from '@shared/domain/scriptRuns';
import { Card } from '@theme/components/ui/card';
import { FieldError } from '@theme/components/ui/field';
import VarText from '@theme/components/VarText.vue';
import { type MessageKey, t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import type { TextPart } from '@theme/varText';
import { formatCurrency } from '@workbench/behaviour/format';
import { computed } from 'vue';

const props = defineProps<{ preview: ScriptRunPreview }>();

const MODE_LABEL = {
  kira: 'automations.dir.kira.label',
  fixed: 'automations.dir.chosen.label',
  home: 'automations.dir.home.label',
} as const;
const dir = computed(() => props.preview.dir);
const folderKey = computed<MessageKey>(() => {
  const d = dir.value;
  if (d.mode !== 'worktree') return 'automations.run.folder.label';
  return d.pending ? 'automations.run.worktreePending.label' : 'automations.run.worktree.label';
});
const folderParams = computed(() => {
  const d = dir.value;
  return d.mode === 'worktree' ? undefined : { mode: t(MODE_LABEL[d.mode]) };
});
function envParts(e: ScriptRunPreview['env'][number]): TextPart[] {
  return [
    `${e.name}=`,
    ...(e.secret
      ? [e.saved ? `•••• (${t('automations.params.secretValue.saved.status')})` : '••••']
      : [{ name: e.fromVar, value: e.value }]),
  ];
}
</script>

<template>
  <Card class="text-kira-sm" data-testid="run-preview">
    <div data-testid="run-folder">
      <CatalogText :k="folderKey" :params="folderParams">
        <template v-if="dir.mode === 'worktree'" #branch
          ><VarText :parts="[{ name: 'branch', value: dir.branch, kind: 'branch' }]" /></template
        >
        <template #folder
          ><VarText :parts="[{ name: 'folder', value: dir.path, kind: 'path' }]" /></template
        >
      </CatalogText>
    </div>
    <template v-if="preview.kind === 'smart'">
      <div class="text-muted-foreground" data-testid="run-limits">
        {{
          t('automations.run.limits.label', {
            model: preview.model,
            budget: formatCurrency(preview.maxBudgetUsd),
            timeout: preview.timeout,
          })
        }}
      </div>
      <div class="font-data" data-testid="run-tools">--tools {{ preview.tools.join(',') }}</div>
      <div class="font-data break-words" data-testid="run-allowed-tools"
        >--allowedTools {{ preview.allowedTools.join(' ') }}</div
      >
      <div v-if="preview.mcpServers.length > 0" data-testid="run-mcp">{{
        t('automations.run.mcpServers.label', { servers: preview.mcpServers.join(', ') })
      }}</div>
    </template>
    <div v-else class="font-data whitespace-pre-wrap break-words" data-testid="run-command">{{
      preview.command
    }}</div>
    <div v-if="preview.env.length > 0" class="flex flex-col font-data" data-testid="run-env">
      <div v-for="e in preview.env" :key="e.name" data-testid="run-env-row"
        ><VarText :parts="envParts(e)" /></div
      >
    </div>
    <FieldError v-if="preview.secretIssue" data-testid="run-secret-issue">{{
      preview.secretIssue
    }}</FieldError>
    <div v-if="preview.kind === 'smart'" class="text-muted-foreground" data-testid="run-isolated">
      {{ t('automations.run.isolated.body') }}
    </div>
  </Card>
</template>
