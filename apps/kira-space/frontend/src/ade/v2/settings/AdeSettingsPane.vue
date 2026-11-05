<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { useTemplateRef } from 'vue';
import { control } from '../../../bridge/control';
import { useAgentFileEditor } from '../agents/useAgentFileEditor';
import AdeSettingsResult from './AdeSettingsResult.vue';
import AdeSettingsSecrets from './AdeSettingsSecrets.vue';
import { useAgentSettings, useSaveAgentSettings } from './queries';

// The agent settings: Markdown every agent Kira starts gets as extra system prompt.
const query = useAgentSettings();
const save = useSaveAgentSettings();

const {
  view,
  draft,
  dirty,
  saveError,
  conflict,
  bytes,
  actionable,
  over,
  saveNow,
  discard,
  reload,
  overwrite,
  onInput,
} = useAgentFileEditor(query, save);

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void saveNow());
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto p-4" data-testid="ade-settings">
    <div v-if="view" ref="root" class="grid grid-cols-1 items-start gap-6 xl:grid-cols-2">
      <div class="flex min-w-0 flex-col gap-3">
        <ConfigFileBar
          v-if="view.exists"
          :path="view.path"
          :error="view.problem"
          :open="() => control.configFilesOpen({ folder: view!.folder, file: view!.file })"
          :reveal="() => control.configFilesReveal({ folder: view!.folder, file: view!.file })"
          :about="() => control.configFilesOpen({ folder: view!.folder, file: 'AGENTS.md' })"
        />
        <div v-else class="text-kira-sm text-subtle" data-testid="ade-settings-none">
          {{ t('space.ade.settings.none.hint', { path: view.path }) }}
        </div>
        <div
          v-if="view.lastGood"
          class="text-kira-sm text-subtle"
          data-testid="ade-settings-last-good"
        >
          {{ t('space.ade.context.lastGood.hint') }}
        </div>
        <div class="flex flex-col gap-1 text-kira-sm text-subtle" data-testid="ade-settings-note">
          <span>{{ t('space.ade.settings.applies.hint') }}</span>
        </div>
        <div
          v-if="!view.editable"
          class="text-kira-sm text-subtle"
          data-testid="ade-settings-read-only"
        >
          {{ t('space.ade.context.readOnly.hint') }}
        </div>
        <template v-else>
          <CodeTextEditor
            :model-value="draft"
            language="yaml"
            :ariaLabel="t('space.ade.settings.field.aria')"
            class="h-64"
            data-testid="ade-settings-text"
            @update:model-value="onInput"
            @save="saveNow"
            @submit="saveNow"
          />
          <div class="flex flex-wrap items-center gap-2">
            <span
              class="min-w-0 flex-1 text-kira-sm"
              :class="over ? 'text-tone-red' : 'text-subtle'"
              data-testid="ade-settings-size"
            >
              {{
                over
                  ? t('space.ade.settings.overSize.error', { max: view.maxBytes })
                  : t('space.ade.context.size.status', { used: bytes, max: view.maxBytes })
              }}
            </span>
            <Button
              variant="dialog"
              size="kira-lg"
              :disabled="!actionable"
              data-testid="ade-settings-discard"
              @click="discard"
            >
              {{ t('common.action.discard.label') }}
            </Button>
            <Button
              variant="dialog-primary"
              size="kira-lg"
              :disabled="!dirty"
              :pending="save.isPending.value"
              data-testid="ade-settings-save"
              @click="saveNow"
            >
              {{ t('common.action.save.label') }}
            </Button>
          </div>
        </template>
        <Alert v-if="saveError" variant="error" data-testid="ade-settings-save-error">
          <AlertDescription>{{ saveError }}</AlertDescription>
          <AlertAction v-if="conflict">
            <Button variant="link" size="inline" data-testid="ade-settings-reload" @click="reload">
              {{ t('common.action.refresh.label') }}
            </Button>
            <Button
              variant="link"
              size="inline"
              data-testid="ade-settings-overwrite"
              @click="overwrite"
            >
              {{ t('space.ade.workflows.conflict.overwrite.label') }}
            </Button>
          </AlertAction>
        </Alert>
        <AdeSettingsSecrets :view="view" />
      </div>
      <AdeSettingsResult :providers="view.providers" />
    </div>
  </div>
</template>
