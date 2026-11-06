<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import SettingsShell from '@workbench/components/SettingsShell.vue';
import PromptsPane from '@workbench/llmPrompts/PromptsPane.vue';
import { useSettingsDeepLinkReset } from '@workbench/settings/useSettingsDeepLinkReset';
import { useLlmPromptsShellExtra } from '@workbench/state/llmPromptDrafts';
import { computed } from 'vue';
import { type Section, sections, useSettingsStore } from '../state/settings';
import { defaultSettings, type SettingsPatch } from '../state/settingsDomain';
import ApiPane from '../studio/workbench/settings/ApiPane.vue';
import CachePane from '../studio/workbench/settings/CachePane.vue';
import DatabaseMcpPane from '../studio/workbench/settings/DatabaseMcpPane.vue';
import DataPane from '../studio/workbench/settings/DataPane.vue';
import AdvancedPane from './settings/AdvancedPane.vue';
import AgentPane from './settings/AgentPane.vue';
import AppearancePane from './settings/AppearancePane.vue';
import CapturePane from './settings/CapturePane.vue';
import CliToolsPane from './settings/CliToolsPane.vue';
import FileTypesPane from './settings/FileTypesPane.vue';
import GitPane from './settings/GitPane.vue';
import MemoryPane from './settings/MemoryPane.vue';
import MobileAccessPane from './settings/MobileAccessPane.vue';
import UpdatesPane from './settings/UpdatesPane.vue';

const emit = defineEmits<{ close: [] }>();

const settingsStore = useSettingsStore();
const promptsExtra = useLlmPromptsShellExtra();
const { info: agentInfo } = useAgentInfo();

const sectionLabels = computed<Record<Section, string>>(() => ({
  Appearance: t('common.appearance.label'),
  Git: t('common.module.git.label'),
  'CLI tools': t('space.settings.nav.cliTools.label'),
  'File types': t('space.settings.nav.fileTypes.label'),
  Capture: t('space.settings.nav.capture.label'),
  Updates: t('space.settings.nav.updates.label'),
  'Mobile access': t('space.settings.nav.mobileAccess.label'),
  Agent: agentInfo.value.productLabel,
  Memory: t('common.module.memory.label'),
  Data: t('studio.settings.nav.data.label'),
  Cache: t('studio.settings.nav.cache.label'),
  Api: t('studio.mode.api.label'),
  'Database MCP': t('studio.settings.nav.databaseMcp.label'),
  Prompts: t('llmPrompts.nav.label'),
  Advanced: t('common.advanced.label'),
}));

// §10.1: a later plain open (the title bar's gear icon) must not inherit a deep link this instance
// was opened with.
useSettingsDeepLinkReset(settingsStore);

const save = (patch: SettingsPatch): Promise<void> => settingsStore.patchSettings(patch);
</script>

<template>
  <SettingsShell
    :section-labels="sectionLabels"
    :sections="sections"
    :initial-section="settingsStore.settingsSection ?? undefined"
    :defaults="{
      appearance: defaultSettings.appearance,
      advanced: defaultSettings.advanced,
      git: defaultSettings.git,
      agent: defaultSettings.agent,
      memory: defaultSettings.memory,
      capture: defaultSettings.capture,
      tracking: defaultSettings.tracking,
      data: defaultSettings.data,
      cache: defaultSettings.cache,
      api: defaultSettings.api,
      ade: {
        headlessSettingSources: defaultSettings.ade.headlessSettingSources,
      },
    }"
    :current="{
      appearance: settingsStore.appearance,
      advanced: settingsStore.advanced,
      git: settingsStore.git,
      agent: settingsStore.agent,
      memory: settingsStore.memory,
      capture: settingsStore.capture,
      tracking: settingsStore.tracking,
      data: settingsStore.data,
      cache: settingsStore.cache,
      api: settingsStore.api,
      ade: {
        headlessSettingSources: settingsStore.ade.headlessSettingSources,
      },
    }"
    :save="save"
    :extra="promptsExtra"
    @close="emit('close')"
  >
    <template #pane="s">
      <AppearancePane
        :active="s.activeSection === 'Appearance'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <GitPane
        :active="s.activeSection === 'Git'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <CliToolsPane
        :active="s.activeSection === 'CLI tools'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <FileTypesPane
        :active="s.activeSection === 'File types'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <CapturePane
        :active="s.activeSection === 'Capture'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <UpdatesPane
        :active="s.activeSection === 'Updates'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <MobileAccessPane
        :active="s.activeSection === 'Mobile access'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <AgentPane
        :active="s.activeSection === 'Agent'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <MemoryPane
        :active="s.activeSection === 'Memory'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <DataPane
        :active="s.activeSection === 'Data'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <CachePane
        :active="s.activeSection === 'Cache'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <ApiPane
        :active="s.activeSection === 'Api'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <DatabaseMcpPane
        :active="s.activeSection === 'Database MCP'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
      <PromptsPane :active="s.activeSection === 'Prompts'" />
      <AdvancedPane
        :active="s.activeSection === 'Advanced'"
        :draft="s.draft"
        :is-at-default="s.isAtDefault"
        :reset-leaf="s.resetLeaf"
        :register-field-error="s.registerFieldError"
      />
    </template>

    <template #footer="f">
      <div class="mr-auto min-w-0">
        <FieldError v-if="f.saveError" data-testid="settings-save-error">{{
          f.saveError
        }}</FieldError>
        <span
          v-else
          class="text-subtle text-kira-sm leading-normal"
          data-testid="settings-footer-status"
          >{{
            f.isDirty ? t('common.state.unsavedChanges.status') : ''
          }}</span
        >
      </div>
      <Button variant="dialog" size="kira-lg" data-testid="settings-cancel" @click="f.onDismiss">{{
        t('common.action.cancel.label')
      }}</Button>
      <Button
        variant="dialog-primary"
        size="kira-lg"
        :disabled="!f.isValid"
        data-testid="settings-save"
        @click="f.onSave"
      >
        {{ t('common.action.save.label') }}
      </Button>
    </template>
  </SettingsShell>
</template>
