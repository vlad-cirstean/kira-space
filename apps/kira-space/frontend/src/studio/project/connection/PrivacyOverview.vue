<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Card } from '@theme/components/ui/card';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';

// The tab's model in three lines: what the agent sees, what happens to new columns, what you see.
defineProps<{
  ruleCount: number;
  /** Column review exists for this connection kind. */
  reviewApplies: boolean;
  reviewOn: boolean;
  mcpEnabled: boolean;
}>();
</script>

<template>
  <Card class="gap-1" data-testid="connection-privacy-overview">
    <div class="text-fg font-medium">
      {{ t('studio.project.connectionDialog.privacy.overview.title') }}
    </div>
    <p class="text-kira-sm" data-testid="connection-privacy-overview-rules">
      {{
        ruleCount > 0
          ? t('studio.settings.dbMcp.masked.status', { count: ruleCount })
          : t('studio.project.connectionDialog.privacy.overviewNone.status')
      }}
    </p>
    <p v-if="reviewApplies" class="text-kira-sm" data-testid="connection-privacy-overview-review">
      {{
        t(
          reviewOn
            ? 'studio.project.connectionDialog.privacy.overviewHidden.status'
            : 'studio.project.connectionDialog.privacy.overviewOpen.status',
        )
      }}
    </p>
    <FieldDescription class="w-full">{{
      t('studio.project.connectionDialog.privacy.overviewReal.body')
    }}</FieldDescription>
    <Alert v-if="!mcpEnabled" data-testid="connection-privacy-overview-mcp-off">
      <AlertDescription>{{
        t('studio.project.connectionDialog.privacy.overviewMcpOff.hint')
      }}</AlertDescription>
    </Alert>
  </Card>
</template>
