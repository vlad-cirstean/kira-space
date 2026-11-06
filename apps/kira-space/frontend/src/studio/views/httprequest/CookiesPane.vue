<script setup lang="ts">
import type { HttpCookieWire, HttpResponseWire } from '@shared/domain/http';
import { useMutation } from '@tanstack/vue-query';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { matchesQuery } from '@workbench/behaviour/match';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { useSettingsStore } from '../../state/settings';
import { clearJarCookies, deleteJarCookie } from './cookies';
import { useHttpRequestViewStore } from './state';

// P90 item 2 (§3): one component, two hosts — HttpRequestView.vue's request segment (what the
// jar would send next) and ResponsePane.vue's response segment (what one exchange actually sent
// and received), same list shape either way (§3's own reasoning for keeping this one component).
const props = defineProps<{
  mode: 'request' | 'response';
  tabId: string;
  /** request mode only — the jar listing for `url` (HttpRequestView.vue owns the query, so the
   *  debounced key is computed once and the segment badge reads the same result). */
  url?: string;
  cookies?: HttpCookieWire[];
  error?: Error | null;
  disableCookieJar?: boolean;
  /** response mode only — null when there is no response to show cookies for yet. */
  response?: HttpResponseWire | null;
}>();

const settingsStore = useSettingsStore();
const confirmDialogStore = useConfirmDialogStore();

const httpRequestViewStore = useHttpRequestViewStore();
const filter = computed({
  get: () => httpRequestViewStore.runtime[props.tabId]?.cookieFilter[props.mode] ?? '',
  set: (v: string) => httpRequestViewStore.setCookieFilter(props.tabId, props.mode, v),
});

function matches(c: HttpCookieWire, q: string): boolean {
  return matchesQuery(c.name, q) || matchesQuery(c.value, q);
}

// §3: "Domain=… Path=… Expires=… Secure HttpOnly SameSite=Lax" — attributes with nothing to show
// are skipped rather than printed empty.
function attributeLine(c: HttpCookieWire): string {
  const parts: string[] = [];
  if (c.domain) parts.push(`Domain=${c.domain}`);
  if (c.path) parts.push(`Path=${c.path}`);
  if (c.expires) parts.push(`Expires=${c.expires}`);
  if (c.secure) parts.push('Secure');
  if (c.httpOnly) parts.push('HttpOnly');
  if (c.sameSite) parts.push(`SameSite=${c.sameSite[0].toUpperCase()}${c.sameSite.slice(1)}`);
  return parts.join(' ');
}

// --- request mode ---

const requestCookies = computed(() => props.cookies ?? []);
const filteredRequestCookies = computed(() => {
  return requestCookies.value.filter((c) => matches(c, filter.value));
});

const emit = defineEmits<{ refresh: [] }>();

const removeMutation = useMutation({
  mutationKey: ['httpJarCookies', 'remove'],
  mutationFn: (c: HttpCookieWire) => deleteJarCookie(props.url ?? '', c),
});
const clearMutation = useMutation({
  mutationKey: ['httpJarCookies', 'clear'],
  mutationFn: clearJarCookies,
});
const requestError = computed(
  () => (removeMutation.error.value ?? clearMutation.error.value ?? props.error)?.message ?? null,
);

async function onRemove(c: HttpCookieWire): Promise<void> {
  if (!props.url) return;
  clearMutation.reset();
  await removeMutation.mutateAsync(c).catch(() => {
    // shown through requestError
  });
}
async function onClearAll(): Promise<void> {
  // Go's jar is process-wide: this empties every host, not just this request's URL.
  if (
    !(await confirmDialogStore.confirm({
      title: t('studio.http.cookies.clearConfirm.title'),
      message: [
        t('studio.http.cookies.clearConfirm.body'),
        t('common.confirm.irreversible.body'),
      ].join(' '),
      confirmLabel: t('common.action.clear.label'),
      tone: 'danger',
    }))
  )
    return;
  removeMutation.reset();
  await clearMutation.mutateAsync().catch(() => {
    // shown through requestError
  });
}

function onEditGlobalDefaults(): void {
  settingsStore.openSettingsAt('Api');
}

// --- response mode ---

const sentCookies = computed(() => props.response?.sentCookies ?? []);
const receivedCookies = computed(() => props.response?.receivedCookies ?? []);
const showHopIndex = computed(() => (props.response?.timeline?.hops.length ?? 0) > 1);
</script>

<template>
  <div
    v-if="mode === 'request'"
    class="flex flex-1 min-h-0 flex-col overflow-auto"
    data-testid="http-request-cookies"
  >
    <EmptyState
      v-if="disableCookieJar"
      data-testid="http-cookies-jar-off"
      icon="circle-slash"
      :title="t('studio.http.cookies.jarOff.empty')"
    >
      <Button
        variant="link"
        size="inline"
        class="mt-1"
        data-testid="http-cookies-edit-defaults"
        @click="onEditGlobalDefaults"
      >
        {{ t('studio.http.settings.editGlobal.label') }}
      </Button>
    </EmptyState>
    <template v-else>
      <div class="flex items-center gap-1">
        <SearchField
          v-model="filter"
          :placeholder="t('studio.http.cookies.filter.placeholder')"
          data-testid="http-cookies-filter"
        />
        <Button
          variant="dialog"
          size="kira"
          class="shrink-0"
          :disabled="requestCookies.length === 0"
          :pending="clearMutation.isPending.value"
          data-testid="http-cookies-clear-all"
          @click="onClearAll"
        >
          {{ t('common.action.clearAll.label') }}
        </Button>
      </div>
      <span
        v-if="filter.trim()"
        class="text-kira-sm text-subtle px-1.5 pt-1 pb-0"
        data-testid="http-cookies-filtered-count"
      >
        {{
          t('studio.http.cookies.count.status', {
            shown: filteredRequestCookies.length,
            count: requestCookies.length,
          })
        }}
      </span>
      <Alert strip v-if="requestError" variant="error" data-testid="http-cookies-error">
        <AlertDescription>{{ requestError }}</AlertDescription>
      </Alert>
      <div
        v-if="requestCookies.length > 0"
        class="flex flex-1 min-h-0 flex-col gap-0.5 overflow-auto p-1.5"
      >
        <div
          v-for="(c, i) in filteredRequestCookies"
          :key="`${c.name}|${c.domain}|${c.path}|${i}`"
          class="flex text-kira-sm items-start justify-between gap-1"
        >
          <div class="flex min-w-0 flex-1 flex-col">
            <span class="text-muted-foreground shrink-0 min-w-40 font-data">{{ c.name }}</span>
            <span class="wrap-anywhere font-data">{{ c.value }}</span>
            <span v-if="attributeLine(c)" class="text-muted-foreground text-kira-sm font-data">{{
              attributeLine(c)
            }}</span>
          </div>
          <TooltipIconButton
            :icon="ICON.close"
            :label="t('common.action.remove.label')"
            :aria-label="t('studio.http.cookies.remove.aria')"
            :data-testid="`http-cookies-remove-${c.name}-${c.domain}${c.path}`"
            @click="onRemove(c)"
          />
        </div>
      </div>
      <EmptyState
        v-else
        data-testid="http-cookies-empty"
        icon="symbol-key"
        :title="t('studio.http.cookies.none.empty')"
      >
        <Button
          variant="link"
          size="inline"
          class="mt-1"
          data-testid="http-cookies-retry"
          @click="emit('refresh')"
        >
          {{ t('common.action.refresh.label') }}
        </Button>
      </EmptyState>
    </template>
  </div>

  <div
    v-else
    class="flex flex-1 min-h-0 flex-col overflow-auto"
    data-testid="http-response-cookies"
  >
    <template v-if="response">
      <template v-if="sentCookies.length > 0 || receivedCookies.length > 0">
        <div v-if="sentCookies.length > 0" class="pt-1.5">
          <SectionHeading :label="t('common.sent.status')" />
          <div class="flex flex-1 min-h-0 flex-col gap-0.5 overflow-auto p-1.5">
            <div
              v-for="(c, i) in sentCookies"
              :key="`sent-${i}`"
              class="flex text-kira-sm items-start justify-between gap-1"
            >
              <div class="flex min-w-0 flex-1 flex-col">
                <span class="text-muted-foreground shrink-0 min-w-40 font-data">{{ c.name }}</span>
                <span class="wrap-anywhere font-data">{{ c.value }}</span>
                <span class="text-muted-foreground text-kira-sm font-data">
                  <template v-if="showHopIndex">{{
                    t('studio.http.cookies.hop.label', { hop: c.hop })
                  }}</template>
                  <template v-if="showHopIndex && attributeLine(c)"> · </template>
                  {{ attributeLine(c) }}
                </span>
              </div>
            </div>
          </div>
        </div>
        <div v-if="receivedCookies.length > 0" class="pt-1.5">
          <SectionHeading :label="t('studio.http.cookies.received.label')" />
          <div class="flex flex-1 min-h-0 flex-col gap-0.5 overflow-auto p-1.5">
            <div
              v-for="(c, i) in receivedCookies"
              :key="`received-${i}`"
              class="flex text-kira-sm items-start justify-between gap-1"
            >
              <div class="flex min-w-0 flex-1 flex-col">
                <span class="text-muted-foreground shrink-0 min-w-40 font-data">{{ c.name }}</span>
                <span class="wrap-anywhere font-data">{{ c.value }}</span>
                <span class="text-muted-foreground text-kira-sm font-data">
                  <template v-if="showHopIndex">{{
                    t('studio.http.cookies.hop.label', { hop: c.hop })
                  }}</template>
                  <template v-if="showHopIndex && attributeLine(c)"> · </template>
                  {{ attributeLine(c) }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </template>
      <EmptyState
        v-else
        data-testid="http-response-cookies-empty"
        icon="symbol-key"
        :title="t('studio.http.cookies.noneReceived.empty')"
      />
    </template>
    <EmptyState v-else icon="arrow-right" :title="t('studio.common.responseEmpty.empty')" />
  </div>
</template>
