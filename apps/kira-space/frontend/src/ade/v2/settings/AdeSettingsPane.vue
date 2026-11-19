<script setup lang="ts">
import { t } from '@theme/i18n';
import SaveBar from '@workbench/components/SaveBar.vue';
import ConfigFileProblem from '@workbench/config/ConfigFileProblem.vue';
import SaveConflictAlert from '@workbench/config/SaveConflictAlert.vue';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { useTemplateRef } from 'vue';
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
  saving,
  saveError,
  conflict,
  bytes,
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
  <div class="min-h-0 flex-1 overflow-auto p-3" data-testid="ade-settings">
    <div v-if="view" ref="root" class="grid grid-cols-1 items-start gap-4 xl:grid-cols-2">
      <div class="flex min-w-0 flex-col gap-3">
        <div v-if="!view.exists" class="text-kira-sm text-subtle" data-testid="ade-settings-none">
          {{ t('space.ade.settings.none.hint') }}
        </div>
        <ConfigFileProblem v-else :error="view.problem" />
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
          {{ t('common.content.readOnly.hint') }}
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
          <SaveBar
            :dirty="dirty"
            :saving="saving"
            :can-save="dirty"
            testid-prefix="ade-settings"
            @save="saveNow"
            @discard="discard"
          >
            <template #start>
              <span :class="over && 'text-error'" data-testid="ade-settings-size">
                {{
                  over
                    ? t('space.ade.settings.overSize.error', { max: view.maxBytes })
                    : t('space.ade.context.size.status', { used: bytes, max: view.maxBytes })
                }}
              </span>
            </template>
          </SaveBar>
        </template>
        <SaveConflictAlert
          v-if="saveError"
          :error="saveError"
          :conflict="conflict"
          testid-prefix="ade-settings"
          @reload="reload"
          @overwrite="overwrite"
        />
        <AdeSettingsSecrets :view="view" />
      </div>
      <AdeSettingsResult :providers="view.providers" />
    </div>
  </div>
</template>
