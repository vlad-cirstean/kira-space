<script setup lang="ts">
import type { ApiEnvironmentFileModel, ApiFileVariable } from '@shared/domain/apiConfig';
import InlineState from '@theme/components/InlineState.vue';
import { Badge } from '@theme/components/ui/badge';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import {
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
} from '@theme/components/ui/tags-input';
import { t } from '@theme/i18n';
import { useId } from 'vue';
import { useSettingsStore } from '../../state/settings';
import ReviewChoiceField from '../components/ReviewChoiceField.vue';
import { cleanVariable, defaultAgentHosts } from './variablesForm';

// Which variables of this environment an agent may use through the API request tools, and where a
// secret may go. Edits land in the dialog's draft, so the YAML tab shows them live.
const props = defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const env = defineModel<ApiEnvironmentFileModel>({ required: true });
const readsId = useId();
const settingsStore = useSettingsStore();

// A class left out of agentReview is inherit; the object is dropped once both are.
const REVIEW_CLASSES = [
  { cls: 'read', label: 'studio.api.agentAccess.reviewRead.label' },
  { cls: 'write', label: 'studio.api.agentAccess.reviewWrite.label' },
] as const;
type ReviewClass = (typeof REVIEW_CLASSES)[number]['cls'];

function reviewOf(cls: ReviewClass): string {
  return env.value.agentReview?.[cls] ?? 'inherit';
}

function setReview(cls: ReviewClass, value: string): void {
  const next: Record<string, string> = {};
  for (const c of REVIEW_CLASSES) {
    const v = c.cls === cls ? value : reviewOf(c.cls);
    if (v !== 'inherit') next[c.cls] = v;
  }
  env.value.agentReview =
    Object.keys(next).length > 0 ? (next as ApiEnvironmentFileModel['agentReview']) : undefined;
}

function setVariable(index: number, next: ApiFileVariable): void {
  env.value.variables = (env.value.variables ?? []).map((v, i) =>
    i === index ? cleanVariable(next) : v,
  );
}

function toggle(index: number, v: ApiFileVariable, on: boolean): void {
  if (!on) {
    setVariable(index, { ...v, agent: false, agentHosts: undefined });
    return;
  }
  const hosts =
    v.secret && !v.agentHosts?.length ? defaultAgentHosts(env.value.variables ?? []) : [];
  setVariable(index, {
    ...v,
    agent: true,
    agentHosts: v.secret ? [...(v.agentHosts ?? []), ...hosts] : undefined,
  });
}

function setHosts(index: number, v: ApiFileVariable, hosts: string[]): void {
  setVariable(index, { ...v, agentHosts: hosts.map((h) => String(h).trim()).filter(Boolean) });
}

function hostsError(index: number, v: ApiFileVariable): string | undefined {
  if (v.secret && v.agent && !v.agentHosts?.length) {
    return t('studio.api.agentAccess.hostsRequired.error');
  }
  return (
    props.errorFor(`/variables/${index}/agentHosts`) ?? props.errorFor(`/variables/${index}/agent`)
  );
}
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="api-section-agent">
    <FieldDescription>{{ t('studio.api.agentAccess.intro.body') }}</FieldDescription>
    <Field orientation="horizontal">
      <Checkbox
        :id="readsId"
        :model-value="env.agentAutoApproveReads ?? false"
        data-testid="api-agent-auto-reads"
        @update:model-value="(v) => (env.agentAutoApproveReads = v === true)"
      />
      <FieldLabel :for="readsId">{{
        t('studio.api.agentAccess.autoApproveReads.label')
      }}</FieldLabel>
    </Field>
    <FieldError v-if="props.errorFor('/agentAutoApproveReads')">{{
      props.errorFor('/agentAutoApproveReads')
    }}</FieldError>

    <FieldSet data-testid="api-agent-review">
      <FieldLegend>{{ t('studio.api.agentAccess.reviewLegend.label') }}</FieldLegend>
      <FieldDescription>{{ t('studio.api.agentAccess.reviewIntro.body') }}</FieldDescription>
      <ReviewChoiceField
        v-for="c in REVIEW_CLASSES"
        :key="c.cls"
        :label="t(c.label)"
        :app-on="settingsStore.dbMcp.apiReviewResponses"
        :testid="`api-agent-review-${c.cls}`"
        :model-value="reviewOf(c.cls)"
        @update:model-value="(v) => setReview(c.cls, v)"
      />
    </FieldSet>
    <FieldError v-if="props.errorFor('/agentReview')">{{
      props.errorFor('/agentReview')
    }}</FieldError>

    <ul class="m-0 flex list-none flex-col gap-2 p-0" data-testid="api-agent-variables">
      <li
        v-for="(v, i) in env.variables ?? []"
        :key="`${i}-${v.name}`"
        class="flex flex-col gap-1"
        :data-testid="`api-agent-row-${v.name}`"
      >
        <div class="flex items-center gap-2">
          <Checkbox
            :model-value="v.agent ?? false"
            :aria-label="t('studio.api.agentAccess.available.aria', { name: v.name })"
            :data-testid="`api-agent-available-${v.name}`"
            @update:model-value="(on) => toggle(i, v, on === true)"
          />
          <span class="min-w-0 break-all font-data text-kira-md">{{ v.name }}</span>
          <Badge v-if="v.secret">{{ t('common.secret.label') }}</Badge>
        </div>
        <div v-if="v.secret && v.agent" class="flex flex-col gap-1 pl-8">
          <TagsInput
            :model-value="v.agentHosts ?? []"
            :aria-invalid="hostsError(i, v) ? true : undefined"
            add-on-blur
            :data-testid="`api-agent-hosts-${v.name}`"
            @update:model-value="(hosts) => setHosts(i, v, hosts.map(String))"
          >
            <TagsInputItem v-for="host in v.agentHosts ?? []" :key="host" :value="host">
              <TagsInputItemText />
              <TagsInputItemDelete />
            </TagsInputItem>
            <TagsInputInput
              :placeholder="t('studio.api.agentAccess.hosts.placeholder')"
              :aria-label="t('studio.api.agentAccess.hosts.aria', { name: v.name })"
            />
          </TagsInput>
          <FieldError v-if="hostsError(i, v)" :data-testid="`api-agent-hosts-error-${v.name}`">{{
            hostsError(i, v)
          }}</FieldError>
          <FieldDescription>{{ t('studio.api.agentAccess.hosts.hint') }}</FieldDescription>
        </div>
      </li>
    </ul>
    <InlineState kind="empty" v-if="!(env.variables ?? []).length" data-testid="api-agent-empty">{{
      t('studio.api.config.variables.empty')
    }}</InlineState>
  </div>
</template>
