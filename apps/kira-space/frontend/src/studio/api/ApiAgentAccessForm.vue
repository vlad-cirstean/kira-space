<script setup lang="ts">
import type { ApiEnvironmentFileModel, ApiFileVariable } from '@shared/domain/apiConfig';
import { Badge } from '@theme/components/ui/badge';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import {
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
} from '@theme/components/ui/tags-input';
import { t } from '@theme/i18n';
import { useId } from 'vue';
import { cleanVariable, defaultAgentHosts } from './variablesForm';

// Which variables of this environment an agent may use through the API request tools, and where a
// secret may go. Edits land in the dialog's draft, so the YAML tab shows them live.
const props = defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const env = defineModel<ApiEnvironmentFileModel>({ required: true });
const readsId = useId();

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
    <div class="flex items-center gap-2 text-kira-md">
      <Checkbox
        :id="readsId"
        :model-value="env.agentAutoApproveReads ?? false"
        data-testid="api-agent-auto-reads"
        @update:model-value="(v) => (env.agentAutoApproveReads = v === true)"
      />
      <Label :for="readsId">{{ t('studio.api.agentAccess.autoApproveReads.label') }}</Label>
    </div>
    <FieldError v-if="props.errorFor('/agentAutoApproveReads')">{{
      props.errorFor('/agentAutoApproveReads')
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
    <FieldDescription v-if="!(env.variables ?? []).length" data-testid="api-agent-empty">{{
      t('studio.api.config.variables.empty')
    }}</FieldDescription>
  </div>
</template>
