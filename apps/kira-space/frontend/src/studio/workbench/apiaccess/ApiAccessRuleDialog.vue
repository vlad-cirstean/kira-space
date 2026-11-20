<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import {
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
} from '@theme/components/ui/tags-input';
import { t } from '@theme/i18n';
import { computed, ref, useId, watch } from 'vue';
import type { ApiRule } from '../../../state/settingsDomain';
import { API_METHODS, isHostPattern } from './apiAccessRules';

// Add or edit one API access rule. The parent owns the write; this form only builds the rule.
const props = defineProps<{ rule?: ApiRule; pending: boolean; error: string | null }>();
const open = defineModel<boolean>('open', { required: true });
const emit = defineEmits<{ save: [rule: ApiRule] }>();

const effectItems = computed(() => [
  {
    value: 'allow',
    label: t('studio.project.connectionDialog.mcp.allow.label'),
    testid: 'api-rule-effect-allow',
  },
  { value: 'deny', label: t('common.action.deny.label'), testid: 'api-rule-effect-deny' },
]);
const anyMethodId = useId();
const methodsId = useId();
const effect = ref<ApiRule['effect']>('allow');
const hosts = ref<string[]>([]);
const anyMethod = ref(false);
const methods = ref<string[]>([]);

watch(
  open,
  (isOpen) => {
    if (!isOpen) return;
    const r = props.rule;
    effect.value = r?.effect ?? 'allow';
    hosts.value = [...(r?.hosts ?? [])];
    anyMethod.value = r ? r.methods.includes('*') : false;
    methods.value = r && !anyMethod.value ? [...r.methods] : [];
    initial.value = draftJson.value;
  },
  { immediate: true },
);

const initial = ref('');
const draftJson = computed(() =>
  JSON.stringify([effect.value, hosts.value, anyMethod.value, methods.value]),
);
const dirty = computed(() => draftJson.value !== initial.value);

const badHost = computed(() => hosts.value.find((h) => !isHostPattern(h)));
const valid = computed(
  () =>
    hosts.value.length > 0 &&
    hosts.value.length <= 20 &&
    !badHost.value &&
    (anyMethod.value || methods.value.length > 0),
);

function setMethod(m: string, on: boolean): void {
  methods.value = on ? [...methods.value, m] : methods.value.filter((x) => x !== m);
}

function onSave(): void {
  if (!valid.value) return;
  emit('save', {
    effect: effect.value,
    hosts: [...hosts.value],
    methods: anyMethod.value ? ['*'] : API_METHODS.filter((m) => methods.value.includes(m)),
  });
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent size="lg" :dirty="dirty" data-testid="api-rule-dialog">
      <DialogHeader close-testid="api-rule-close">
        <DialogTitle>{{ t('studio.settings.dbMcp.apiRules.dialog.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Field>
          <FieldLabel>{{ t('studio.settings.dbMcp.apiRules.effect.label') }}</FieldLabel>
          <SecondaryTabs
            v-model="effect"
            variant="choice"
            size="kira-lg"
            :aria-label="t('studio.settings.dbMcp.apiRules.effect.label')"
            :items="effectItems"
            data-testid="api-rule-effect"
          />
          <FieldDescription data-testid="api-rule-effect-hint">{{
            t('studio.settings.dbMcp.apiRules.effect.hint')
          }}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel>{{ t('studio.settings.dbMcp.apiRules.hosts.label') }}</FieldLabel>
          <TagsInput
            v-model="hosts"
            add-on-blur
            :aria-invalid="badHost ? true : undefined"
            data-testid="api-rule-hosts"
          >
            <TagsInputItem v-for="host in hosts" :key="host" :value="host">
              <TagsInputItemText />
              <TagsInputItemDelete />
            </TagsInputItem>
            <TagsInputInput
              :placeholder="t('studio.settings.dbMcp.apiRules.hosts.example')"
              :aria-label="t('studio.settings.dbMcp.apiRules.hosts.label')"
            />
          </TagsInput>
          <FieldError v-if="badHost" data-testid="api-rule-host-error">{{
            t('studio.settings.dbMcp.apiRules.hostInvalid.error', { host: badHost })
          }}</FieldError>
          <FieldDescription>{{ t('studio.settings.dbMcp.apiRules.hosts.hint') }}</FieldDescription>
          <FieldDescription data-testid="api-rule-host-forms">{{
            t('studio.settings.dbMcp.apiRules.hostForms.body')
          }}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel>{{ t('studio.settings.dbMcp.apiRules.methods.label') }}</FieldLabel>
          <div class="flex flex-wrap gap-x-3 gap-y-1.5">
            <Field v-for="m in API_METHODS" :key="m" orientation="horizontal" class="w-auto">
              <Checkbox
                :id="`${methodsId}-${m}`"
                :model-value="methods.includes(m)"
                :disabled="anyMethod"
                :data-testid="`api-rule-method-${m}`"
                @update:model-value="(v) => setMethod(m, v === true)"
              />
              <FieldLabel :for="`${methodsId}-${m}`">{{ m }}</FieldLabel>
            </Field>
          </div>
          <Field orientation="horizontal">
            <Checkbox
              :id="anyMethodId"
              :model-value="anyMethod"
              data-testid="api-rule-any-method"
              @update:model-value="(v) => (anyMethod = v === true)"
            />
            <FieldLabel :for="anyMethodId">{{
              t('studio.settings.dbMcp.apiRules.anyMethod.label')
            }}</FieldLabel>
          </Field>
        </Field>
        <Alert v-if="error" variant="error" data-testid="api-rule-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="pending"
          :disabled="!valid"
          data-testid="api-rule-save"
          @click="onSave"
        >
          {{ t('studio.settings.dbMcp.apiRules.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
