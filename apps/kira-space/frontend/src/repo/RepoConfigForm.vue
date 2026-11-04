<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import { Button } from '@theme/components/ui/button';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { ref, watch } from 'vue';
import { control } from '../bridge/control';
import RepoEnvRow from './RepoEnvRow.vue';
import RepoScriptField from './RepoScriptField.vue';
import type { RepoFileModel, RepoFileWorktree } from './state/repoFormTypes';

// The repo settings file as a form. Edits the file's own keys; the field table (REPO_FIELDS) names
// every leaf written, so the rest of the file (graph, stash, review, ...) is never sent.
const props = defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const file = defineModel<RepoFileModel>({ required: true });

const SETUP_EXAMPLE = 'pnpm install';
const VERSION_EXAMPLE = 'curl -s https://staging.example.com/version';

function worktree(key: keyof Pick<RepoFileWorktree, 'prepare_timeout' | 'base_path'>) {
  return {
    get: () => file.value.worktree?.[key] ?? '',
    set: (v: string) => {
      file.value.worktree = { ...file.value.worktree, [key]: v };
    },
  };
}
const timeout = worktree('prepare_timeout');
const basePath = worktree('base_path');

function setPrepare(v: string): void {
  file.value.worktree = { ...file.value.worktree, prepare_script: v };
}

const parseTargets = (v: string): string[] =>
  v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
// Typing "a, " keeps its separator: the text is local and only follows the model when the list
// changes from elsewhere (a YAML edit).
const targetsText = ref((file.value.integration_branches ?? []).join(', '));
watch(
  () => (file.value.integration_branches ?? []).join('\n'),
  (list) => {
    if (list !== parseTargets(targetsText.value).join('\n')) {
      targetsText.value = (file.value.integration_branches ?? []).join(', ');
    }
  },
);
function onTargets(v: string): void {
  targetsText.value = v;
  file.value.integration_branches = parseTargets(v);
}

async function chooseBasePath(): Promise<void> {
  const chosen = await control.filesChooseFolder(t('space.repo.config.basePath.chooseTitle.title'));
  if (chosen.canceled || !chosen.path) return;
  basePath.set(chosen.path);
}

function setEnv(i: number, env: NonNullable<RepoFileModel['environments']>[number]): void {
  file.value.environments = (file.value.environments ?? []).map((e, k) => (k === i ? env : e));
}
function removeEnv(i: number): void {
  file.value.environments = (file.value.environments ?? []).filter((_, k) => k !== i);
}
function addEnv(): void {
  file.value.environments = [...(file.value.environments ?? []), { name: '' }];
}
</script>

<template>
  <div class="flex flex-col gap-4" data-testid="repo-config-form">
    <FieldSet class="gap-3">
      <FieldLegend>{{ t('space.repo.config.repository.label') }}</FieldLegend>
      <Field>
        <FieldLabel for="repo-nick">{{ t('space.repo.config.nickname.label') }}</FieldLabel>
        <Input
          id="repo-nick"
          :model-value="file.nickname ?? ''"
          class="max-w-60 font-data"
          data-testid="repo-nick"
          @update:model-value="(v) => (file.nickname = String(v))"
        />
        <FieldDescription>{{ t('space.repo.config.nickname.description') }}</FieldDescription>
        <FieldError v-if="props.errorFor('/nickname')" data-testid="repo-nick-error">{{
          props.errorFor('/nickname')
        }}</FieldError>
      </Field>
    </FieldSet>

    <FieldSet class="gap-3">
      <FieldLegend>{{ t('common.module.worktrees.label') }}</FieldLegend>
      <Field>
        <FieldLabel for="repo-prepare">{{ t('space.repo.config.prepare.label') }}</FieldLabel>
        <FieldDescription>
          <CatalogText k="space.repo.config.prepare.description">
            <template #command><code class="font-data">{{ SETUP_EXAMPLE }}</code></template>
          </CatalogText>
        </FieldDescription>
        <RepoScriptField
          id="repo-prepare"
          :model-value="file.worktree?.prepare_script ?? ''"
          :run-ref="file.worktree?.prepare_run"
          :rows="6"
          testid="repo-prepare"
          @update:model-value="setPrepare"
        />
        <FieldError
          v-if="props.errorFor('/worktree/prepare_script')"
          data-testid="repo-prepare-error"
          >{{
            props.errorFor('/worktree/prepare_script')
          }}</FieldError
        >
      </Field>
      <Field orientation="horizontal">
        <FieldLabel for="repo-timeout">{{ t('common.field.timeout.label') }}</FieldLabel>
        <Input
          id="repo-timeout"
          :model-value="timeout.get()"
          class="w-20 font-data"
          data-testid="repo-timeout"
          @update:model-value="(v) => timeout.set(String(v))"
        />
        <FieldError
          v-if="props.errorFor('/worktree/prepare_timeout')"
          data-testid="repo-timeout-error"
          >{{
            props.errorFor('/worktree/prepare_timeout')
          }}</FieldError
        >
      </Field>
      <Field>
        <FieldLabel for="repo-base-path">{{ t('space.repo.config.basePath.label') }}</FieldLabel>
        <FieldDescription>{{ t('space.repo.config.basePath.description') }}</FieldDescription>
        <div class="flex items-center gap-1.5">
          <Input
            id="repo-base-path"
            :model-value="basePath.get()"
            :placeholder="t('space.repo.config.basePath.placeholder')"
            class="min-w-0 flex-1 font-data"
            data-testid="repo-base-path"
            @update:model-value="(v) => basePath.set(String(v))"
          />
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="repo-base-path-choose"
            @click="chooseBasePath"
          >
            {{ t('common.action.choose.label') }}
          </Button>
        </div>
        <FieldError
          v-if="props.errorFor('/worktree/base_path')"
          data-testid="repo-base-path-error"
          >{{
            props.errorFor('/worktree/base_path')
          }}</FieldError
        >
      </Field>
    </FieldSet>

    <FieldSet class="gap-3">
      <FieldLegend>{{ t('common.module.branches.label') }}</FieldLegend>
      <Field>
        <FieldLabel for="repo-targets">{{ t('space.repo.config.targets.label') }}</FieldLabel>
        <FieldDescription>{{ t('space.repo.config.targets.description') }}</FieldDescription>
        <Input
          id="repo-targets"
          :model-value="targetsText"
          :placeholder="t('space.repo.config.targets.example')"
          class="font-data"
          data-testid="repo-targets"
          @update:model-value="(v) => onTargets(String(v))"
        />
        <FieldError
          v-if="props.errorFor('/integration_branches')"
          data-testid="repo-targets-error"
          >{{
            props.errorFor('/integration_branches')
          }}</FieldError
        >
      </Field>
    </FieldSet>

    <FieldSet class="gap-3">
      <FieldLegend>{{ t('space.repo.config.environments.title') }}</FieldLegend>
      <FieldDescription>
        <CatalogText k="space.repo.config.environments.intro.hint">
          <template #command><code class="font-data">{{ VERSION_EXAMPLE }}</code></template>
        </CatalogText>
      </FieldDescription>
      <InlineState v-if="!file.environments?.length" kind="empty" data-testid="repo-env-empty">
        {{ t('common.noEnvironments.empty') }}
      </InlineState>
      <div v-else class="flex flex-col gap-1.5">
        <div
          class="flex items-center gap-1.5 text-kira-sm text-muted-foreground"
          aria-hidden="true"
        >
          <span class="w-32 shrink-0">{{ t('common.field.name.label') }}</span>
          <span class="min-w-0 flex-1">{{ t('space.repo.env.command.label') }}</span>
          <span class="size-6 shrink-0" />
        </div>
        <RepoEnvRow
          v-for="(env, i) in file.environments"
          :key="i"
          :model-value="env"
          :index="i"
          @update:model-value="(v) => setEnv(i, v)"
          @remove="removeEnv(i)"
        />
      </div>
      <FieldError v-if="props.errorFor('/environments')" data-testid="repo-env-error">{{
        props.errorFor('/environments')
      }}</FieldError>
      <Button
        variant="dialog"
        size="kira-lg"
        class="self-start"
        data-testid="repo-add-env"
        @click="addEnv"
      >
        {{ t('space.repo.env.add.label') }}
      </Button>
    </FieldSet>
  </div>
</template>
