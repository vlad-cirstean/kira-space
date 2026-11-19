<script setup lang="ts">
import { PALETTE_COLOR_CHOICES, type PaletteColor } from '@shared/domain/color';
import KeyValueList from '@theme/components/KeyValueList.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Field, FieldError, FieldLabel, FieldSet } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import SwatchPicker from '@theme/SwatchPicker.vue';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, useId } from 'vue';
import type { Repo } from '../ade/v2/wire';
import { useCodeReposStore } from '../state/coderepos';

// What the settings file does not hold: the repo's colour, its path and where it came from.
const props = defineProps<{ repo: Repo }>();

const codeRepos = useCodeReposStore();
const color = computed(() => codeRepos.colorOf(props.repo.codeRepoId));
const chooseColor = useAsyncAction((next: PaletteColor) =>
  codeRepos.setCodeRepoColor(props.repo.codeRepoId, next),
);
const labelId = useId();

const source = computed(() =>
  t('common.dotPair.label', {
    first:
      props.repo.source === 'added'
        ? t('space.repo.config.source.added.status')
        : t('space.repo.config.source.imported.status', { source: props.repo.source }),
    second: props.repo.usedByTasks
      ? t('space.repo.config.source.usedOnPlan.status')
      : t('space.repo.config.source.notOnPlan.status'),
  }),
);
</script>

<template>
  <FieldSet data-testid="repo-config">
    <Field>
      <FieldLabel :id="labelId">{{ t('common.field.colour.label') }}</FieldLabel>
      <SwatchPicker
        :model-value="color"
        :colors="PALETTE_COLOR_CHOICES"
        class="h-control"
        :labelledby="labelId"
        data-testid="repo-color"
        @update:model-value="(v) => chooseColor.run(v as PaletteColor)"
      />
      <FieldError v-if="chooseColor.error.value" data-testid="repo-color-error">{{
        chooseColor.error.value
      }}</FieldError>
    </Field>
    <KeyValueList>
      <dt>{{ t('space.repo.config.repository.label') }}</dt>
      <dd>
        <span class="block min-w-0 font-data" data-testid="repo-full"
          ><TruncatedText :text="repo.name" /></span
        >
      </dd>
      <dt>{{ t('common.path.label') }}</dt>
      <dd>
        <span class="block min-w-0 font-data" data-testid="repo-path"
          ><TruncatedText :text="repo.path" /></span
        >
      </dd>
      <dt>{{ t('common.field.source.label') }}</dt>
      <dd data-testid="repo-source">
        {{ source }}
      </dd>
    </KeyValueList>
  </FieldSet>
</template>
