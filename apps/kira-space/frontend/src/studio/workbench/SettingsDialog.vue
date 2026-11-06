<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import SettingsShell from '@workbench/components/SettingsShell.vue';
import PromptsPane from '@workbench/llmPrompts/PromptsPane.vue';
import { useSettingsDeepLinkReset } from '@workbench/settings/useSettingsDeepLinkReset';
import { useLlmPromptsShellExtra } from '@workbench/state/llmPromptDrafts';
import { computed } from 'vue';
import { type Section, sections, useSettingsStore } from '../state/settings';
import { defaultSettings, type SettingsPatch } from '../state/settingsDomain';
import AdvancedPane from './settings/AdvancedPane.vue';
import ApiPane from './settings/ApiPane.vue';
import AppearancePane from './settings/AppearancePane.vue';
import CachePane from './settings/CachePane.vue';
import DatabaseMcpPane from './settings/DatabaseMcpPane.vue';
import DataPane from './settings/DataPane.vue';

const SETTINGS_DB_PATH = '~/.kira-studio/kira.sqlite';

// The old single-file workbench/SettingsDialog.vue, now a thin composition
// over SettingsShell.vue plus this app's own seven panes (workbench/settings/*.vue — see
// SettingsShell.vue's own file-level comment for what stays app-side and why). Everything
// genuinely app-specific stays here: which six of Settings' sections this dialog edits, the
// dialog's own width/height, the deep-link reset, and the footer markup (its one small, real
// difference from Kira Space's own — see SettingsShell.vue's comment #2).
const emit = defineEmits<{ close: [] }>();

const settingsStore = useSettingsStore();
const promptsExtra = useLlmPromptsShellExtra();

const sectionLabels = computed<Record<Section, string>>(() => ({
  Appearance: t('common.appearance.label'),
  Data: t('studio.settings.nav.data.label'),
  Cache: t('studio.settings.nav.cache.label'),
  Api: t('studio.mode.api.label'),
  'Database MCP': t('studio.settings.nav.databaseMcp.label'),
  Prompts: t('llmPrompts.nav.label'),
  Advanced: t('common.advanced.label'),
}));

// §10.1: a later plain open (the title bar's gear icon, the command palette) must not inherit a
// deep link this instance was opened with.
useSettingsDeepLinkReset(settingsStore);

async function save(patch: SettingsPatch): Promise<void> {
  await settingsStore.patchSettings(patch);
}
</script>

<template>
  <SettingsShell
    :sections="sections"
    :section-labels="sectionLabels"
    :initial-section="settingsStore.settingsSection ?? undefined"
    :defaults="{
      appearance: defaultSettings.appearance,
      data: defaultSettings.data,
      cache: defaultSettings.cache,
      advanced: defaultSettings.advanced,
      api: defaultSettings.api,
    }"
    :current="{
      appearance: settingsStore.appearance,
      data: settingsStore.data,
      cache: settingsStore.cache,
      advanced: settingsStore.advanced,
      api: settingsStore.api,
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
      <span class="mr-auto min-w-0 flex items-center">
        <FieldError v-if="f.saveError" data-testid="settings-save-error">{{
          f.saveError
        }}</FieldError>
        <FieldDescription v-else data-testid="settings-footer-status">
          <CatalogText k="studio.settings.footer.stored.status">
            <template #path><span class="font-data">{{ SETTINGS_DB_PATH }}</span></template>
          </CatalogText>
          <template v-if="f.isDirty">
            · {{ t('common.state.unsavedChanges.status') }}</template
          ></FieldDescription
        >
      </span>
      <Button variant="dialog" size="kira-lg" data-testid="settings-cancel" @click="f.onDismiss">{{
        t('common.action.cancel.label')
      }}</Button>
      <Button
        variant="dialog-primary"
        size="kira-lg"
        data-testid="settings-save"
        :disabled="!f.isValid"
        @click="f.onSave"
      >
        {{ t('common.action.save.label') }}
      </Button>
    </template>
  </SettingsShell>
</template>
