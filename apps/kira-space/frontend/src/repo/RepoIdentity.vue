<script setup lang="ts">
import { PALETTE_COLOR_CHOICES, type PaletteColor } from '@shared/domain/color';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Field, FieldError, FieldSet } from '@theme/components/ui/field';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colourLabel } from '@theme/connColor';
import { t } from '@theme/i18n';
import SwatchRadio from '@theme/SwatchRadio.vue';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import type { Repo } from '../ade/v2/wire';
import { useCodeReposStore } from '../state/coderepos';

type ColourName = Parameters<typeof colourLabel>[0];

// What the settings file does not hold: the repo's colour, its path and where it came from.
const props = defineProps<{ repo: Repo }>();

const codeRepos = useCodeReposStore();
const color = computed(() => codeRepos.colorOf(props.repo.codeRepoId));
const colorError = ref('');
async function chooseColor(next: PaletteColor): Promise<void> {
  colorError.value = '';
  try {
    await codeRepos.setCodeRepoColor(props.repo.codeRepoId, next);
  } catch (err) {
    colorError.value = errorText(err);
  }
}

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
  <FieldSet class="gap-3" data-testid="repo-config">
    <Field>
      <span class="text-muted-foreground">{{ t('common.field.colour.label') }}</span>
      <fieldset
        class="m-0 flex h-control flex-wrap items-center gap-1 border-0 p-0"
        :aria-label="t('common.field.colour.label')"
        data-testid="repo-color"
      >
        <Tooltip v-for="swatch in PALETTE_COLOR_CHOICES" :key="swatch">
          <TooltipTrigger as-child>
            <SwatchRadio
              name="repo-color"
              :value="swatch"
              :color="swatch"
              :checked="color === swatch"
              @change="chooseColor(swatch)"
            />
          </TooltipTrigger>
          <TooltipContent>{{ colourLabel(swatch as ColourName) }}</TooltipContent>
        </Tooltip>
      </fieldset>
      <FieldError v-if="colorError" data-testid="repo-color-error">{{ colorError }}</FieldError>
    </Field>
    <dl class="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-kira-md">
      <dt class="text-muted-foreground">{{ t('space.repo.config.repository.label') }}</dt>
      <dd class="m-0 min-w-0">
        <span class="block min-w-0 font-data" data-testid="repo-full"
          ><TruncatedText :text="repo.name" /></span
        >
      </dd>
      <dt class="text-muted-foreground">{{ t('common.path.label') }}</dt>
      <dd class="m-0 min-w-0">
        <span class="block min-w-0 font-data" data-testid="repo-path"
          ><TruncatedText :text="repo.path" /></span
        >
      </dd>
      <dt class="text-muted-foreground">{{ t('common.field.source.label') }}</dt>
      <dd class="m-0" data-testid="repo-source">
        {{ source }}
      </dd>
    </dl>
  </FieldSet>
</template>
