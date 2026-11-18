<script setup lang="ts">
import DetailHeader from '@theme/components/DetailHeader.vue';
import { Alert } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { storeToRefs } from 'pinia';
import { computed, ref, useId } from 'vue';
import { useAuthStore } from '../state/auth';

const auth = useAuthStore();
const { phase, code, message } = storeToRefs(auth);
const labelId = useId();

// A friendly default the user can edit; the desktop prompt shows it next to the code.
function defaultLabel(): string {
  const data = (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData;
  const ua = navigator.userAgent;
  return (
    data?.platform ||
    (/iPhone/.test(ua)
      ? 'iPhone'
      : /iPad/.test(ua)
        ? 'iPad'
        : /Android/.test(ua)
          ? t('mobile.pair.device.android.label')
          : t('mobile.pair.device.phone.label'))
  );
}
const label = ref(defaultLabel());

const waiting = computed(() => phase.value === 'requesting');
const notice = computed(() => {
  switch (phase.value) {
    case 'denied':
      return t('mobile.pair.notice.denied.error');
    case 'timedOut':
      return t('mobile.pair.notice.timedOut.error');
    case 'revoked':
      return t('mobile.pair.notice.revoked.error');
    case 'expired':
      return t('mobile.pair.notice.expired.error');
    case 'unreachable':
      return t('mobile.pair.notice.unreachable.error');
    default:
      return message.value;
  }
});
</script>

<template>
  <main
    class="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-4 px-4 py-8"
    data-testid="pair-screen"
  >
    <DetailHeader as="h1" flush :title="t('common.product.kiraSpace.label')" />
    <p class="m-0 text-muted-foreground">
      {{ t('mobile.pair.intro.description') }}
    </p>
    <FieldDescription>
      {{ t('mobile.pair.unencrypted.description') }}
    </FieldDescription>

    <template v-if="waiting">
      <Card class="flex flex-col gap-1" data-testid="pair-waiting">
        <span class="text-muted-foreground">{{ t('mobile.pair.waiting.hint') }}</span>
        <span class="font-data text-kira-xl tracking-widest" data-testid="pair-code">{{
          code
        }}</span>
      </Card>
      <Button
        variant="dialog"
        size="kira-lg"
        data-testid="pair-cancel"
        @click="auth.cancelRequest()"
        >{{
          t('common.action.cancel.label')
        }}</Button
      >
    </template>

    <template v-else>
      <Alert v-if="notice" variant="error" data-testid="pair-notice">{{ notice }}</Alert>
      <Field>
        <FieldLabel :for="labelId">{{ t('mobile.pair.name.label') }}</FieldLabel>
        <Input
          :id="labelId"
          v-model="label"
          size="touch"
          maxlength="64"
          autocomplete="off"
          data-testid="pair-label"
        />
      </Field>
      <Button
        variant="dialog-primary"
        size="kira-lg"
        :disabled="!label.trim()"
        data-testid="pair-request"
        @click="auth.requestAccess(label.trim())"
      >
        {{ t('mobile.pair.request.label') }}
      </Button>
    </template>
  </main>
</template>
