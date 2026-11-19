<script setup lang="ts">
import type { RunFile } from '@shared/domain/scripts';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// Whether an agent on the Database MCP server may list and run this script, and how: asking first or
// straight away. Nothing but the description and the run result reaches the agent.
defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const file = defineModel<RunFile>({ required: true });

type Mcp = NonNullable<RunFile['mcp']>;

const mcp = computed<Mcp>(() => file.value.mcp ?? {});
const enabled = computed(() => mcp.value.enabled === true);
const run = computed(() => mcp.value.run ?? 'prompt');
const mode = computed(() => mcp.value.mode ?? 'short');

const runItems = computed(() => [
  { value: 'prompt', label: t('common.mcp.askFirst.label'), testid: 'script-mcp-prompt' },
  { value: 'allow', label: t('automations.form.mcp.allow.label'), testid: 'script-mcp-allow' },
]);
const modeItems = computed(() => [
  { value: 'short', label: t('automations.form.mcp.short.label'), testid: 'script-mcp-short' },
  { value: 'long', label: t('automations.form.mcp.long.label'), testid: 'script-mcp-long' },
]);

function patch(change: Partial<Mcp>): void {
  file.value.mcp = { ...mcp.value, ...change };
}
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="run-tab-mcp">
    <Field orientation="horizontal">
      <Checkbox
        id="script-mcp-enabled"
        :model-value="enabled"
        data-testid="script-mcp-enabled"
        @update:model-value="(v) => patch({ enabled: v === true })"
      />
      <FieldLabel for="script-mcp-enabled">{{ t('automations.form.mcp.enable.label') }}</FieldLabel>
    </Field>
    <FieldDescription>{{ t('automations.form.mcp.enable.hint') }}</FieldDescription>

    <template v-if="enabled">
      <Field>
        <FieldLabel for="script-mcp-description">{{
          t('common.field.description.label')
        }}</FieldLabel>
        <Textarea
          class="min-h-24 max-h-90 resize-y"
          id="script-mcp-description"
          :model-value="mcp.description ?? ''"
          :placeholder="t('automations.form.mcp.description.placeholder')"
          :aria-invalid="errorFor('/mcp/description') ? true : undefined"
          data-testid="script-mcp-description"
          @update:model-value="(v) => patch({ description: String(v) })"
        />
        <FieldDescription>{{ t('automations.form.mcp.description.hint') }}</FieldDescription>
        <FieldError
          v-if="errorFor('/mcp/description')"
          data-testid="script-mcp-description-error"
          >{{
            errorFor('/mcp/description')
          }}</FieldError
        >
      </Field>

      <Field>
        <FieldLabel>{{ t('automations.form.mcp.run.label') }}</FieldLabel>
        <SecondaryTabs
          variant="choice"
          size="kira-lg"
          :aria-label="t('automations.form.mcp.run.label')"
          :items="runItems"
          :model-value="run"
          data-testid="script-mcp-run"
          @update:model-value="(v) => patch({ run: v === 'allow' ? 'allow' : 'prompt' })"
        />
        <FieldError v-if="errorFor('/mcp/run')" data-testid="script-mcp-run-error">{{
          errorFor('/mcp/run')
        }}</FieldError>
      </Field>

      <Field>
        <FieldLabel>{{ t('automations.form.mcp.mode.label') }}</FieldLabel>
        <SecondaryTabs
          variant="choice"
          size="kira-lg"
          :aria-label="t('automations.form.mcp.mode.label')"
          :items="modeItems"
          :model-value="mode"
          data-testid="script-mcp-mode"
          @update:model-value="(v) => patch({ mode: v === 'long' ? 'long' : 'short' })"
        />
        <FieldDescription>{{ t('automations.form.mcp.mode.hint') }}</FieldDescription>
        <FieldError v-if="errorFor('/mcp/mode')" data-testid="script-mcp-mode-error">{{
          errorFor('/mcp/mode')
        }}</FieldError>
      </Field>

      <Field>
        <FieldLabel for="script-mcp-timeout">{{ t('common.field.timeout.label') }}</FieldLabel>
        <Input
          id="script-mcp-timeout"
          :model-value="mcp.timeout ?? ''"
          :placeholder="
            t(
              mode === 'long'
                ? 'automations.form.mcp.longTimeout.example'
                : 'automations.schedule.timeoutExample.example',
            )
          "
          :aria-invalid="errorFor('/mcp/timeout') ? true : undefined"
          class="w-24 font-data"
          data-testid="script-mcp-timeout"
          @update:model-value="(v) => patch({ timeout: String(v) })"
        />
        <FieldDescription>{{ t('automations.schedule.timeoutRange.hint') }}</FieldDescription>
        <FieldError v-if="errorFor('/mcp/timeout')" data-testid="script-mcp-timeout-error">{{
          errorFor('/mcp/timeout')
        }}</FieldError>
      </Field>
    </template>
    <FieldError v-else-if="errorFor('/mcp/enabled')" data-testid="script-mcp-enabled-error">{{
      errorFor('/mcp/enabled')
    }}</FieldError>
  </div>
</template>
