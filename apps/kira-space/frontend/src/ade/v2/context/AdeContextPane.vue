<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import SaveBar from '@workbench/components/SaveBar.vue';
import ConfigFileProblem from '@workbench/config/ConfigFileProblem.vue';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { useTemplateRef } from 'vue';
import { useAgentFileEditor } from '../agents/useAgentFileEditor';
import { useAgentContext, useSaveAgentContext } from './queries';

// The agent context: Markdown every agent Kira starts gets as extra system prompt.
const query = useAgentContext();
const save = useSaveAgentContext();

const {
  view,
  draft,
  dirty,
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
  <div class="min-h-0 flex-1 overflow-auto p-3" data-testid="ade-context">
    <div v-if="view" ref="root" class="flex max-w-4xl flex-col gap-3">
      <div v-if="!view.exists" class="text-kira-sm text-subtle" data-testid="ade-context-none">
        {{ t('space.ade.context.none.hint') }}
      </div>
      <ConfigFileProblem v-else :error="view.problem" />
      <div
        v-if="view.lastGood"
        class="text-kira-sm text-subtle"
        data-testid="ade-context-last-good"
      >
        {{ t('space.ade.context.lastGood.hint') }}
      </div>
      <div class="flex flex-col gap-1 text-kira-sm text-subtle" data-testid="ade-context-note">
        <span>{{ t('space.ade.context.applies.hint') }}</span>
        <span v-for="label in view.keepsOnResume" :key="label" data-testid="ade-context-resume">{{
          t('space.ade.context.resume.hint', { label })
        }}</span>
      </div>
      <div
        v-if="!view.editable"
        class="text-kira-sm text-subtle"
        data-testid="ade-context-read-only"
      >
        {{ t('space.ade.context.readOnly.hint') }}
      </div>
      <template v-else>
        <CodeTextEditor
          :model-value="draft"
          language="markdown"
          :ariaLabel="t('space.ade.context.field.aria')"
          class="h-64"
          data-testid="ade-context-text"
          @update:model-value="onInput"
          @save="saveNow"
          @submit="saveNow"
        />
        <SaveBar
          :dirty="dirty"
          :saving="save.isPending.value"
          :can-save="dirty"
          testid-prefix="ade-context"
          @save="saveNow"
          @discard="discard"
        >
          <template #start>
            <span :class="over && 'text-error'" data-testid="ade-context-size">
              {{
                over
                  ? t('space.ade.context.overSize.error', { max: view.maxBytes })
                  : t('space.ade.context.size.status', { used: bytes, max: view.maxBytes })
              }}
            </span>
          </template>
        </SaveBar>
      </template>
      <Alert v-if="saveError" variant="error" data-testid="ade-context-save-error">
        <AlertDescription>{{ saveError }}</AlertDescription>
        <AlertAction v-if="conflict">
          <Button variant="link" size="inline" data-testid="ade-context-reload" @click="reload">
            {{ t('common.action.refresh.label') }}
          </Button>
          <Button
            variant="link"
            size="inline"
            data-testid="ade-context-overwrite"
            @click="overwrite"
          >
            {{ t('space.ade.workflows.conflict.overwrite.label') }}
          </Button>
        </AlertAction>
      </Alert>
    </div>
  </div>
</template>
