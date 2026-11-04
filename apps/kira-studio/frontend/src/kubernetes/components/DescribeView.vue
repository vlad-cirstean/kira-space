<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import HighlightedText from '@theme/components/HighlightedText.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import SearchField from '@theme/components/SearchField.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { copyText } from '@workbench/behaviour/clipboard';
import { compareNatural, matchesQuery, splitMatches } from '@workbench/behaviour/match';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref } from 'vue';
import { useDescribe } from '../queries';
import type { KubeDescription } from '../wire';

const props = defineProps<{ clusterId: string; kind: string; namespace: string; name: string }>();

const describe = useDescribe({
  clusterId: () => props.clusterId,
  kind: () => props.kind,
  namespace: () => props.namespace,
  name: () => props.name,
});

const filter = ref('');
const needle = computed(() => filter.value.trim().toLowerCase());
const hit = (...texts: string[]): boolean => texts.some((x) => matchesQuery(x, filter.value));
const parts = (text: string) => splitMatches(text, needle.value);

type Pair = [string, string];
const pairs = (o: Record<string, string>): Pair[] =>
  Object.entries(o).sort(([a], [b]) => compareNatural(a, b));

interface Sections {
  overview: Pair[];
  labels: Pair[];
  annotations: Pair[];
  owners: Pair[];
  conditions: KubeDescription['conditions'];
  events: KubeDescription['events'];
  manifest: { n: number; text: string }[];
}

const sections = computed<Sections | null>(() => {
  const d = describe.data.value;
  if (!d) return null;
  const pairHit = ([k, v]: Pair) => hit(k, v);
  const overview: Pair[] = [
    [t('common.field.name.label'), d.name],
    [t('common.field.kind.label'), d.kind],
    [t('studio.common.nodeKind.namespace.label'), d.namespace],
    [t('kubernetes.describe.field.apiVersion.label'), d.apiVersion],
    [t('kubernetes.describe.field.uid.label'), d.uid],
    [t('common.created.label'), d.created],
  ].filter(([, v]) => v !== '') as Pair[];
  return {
    overview: overview.filter(pairHit),
    labels: pairs(d.labels).filter(pairHit),
    annotations: pairs(d.annotations).filter(pairHit),
    owners: d.owners.map((o): Pair => [o.kind, o.name]).filter(pairHit),
    conditions: d.conditions.filter((c) => hit(c.type, c.status, c.reason, c.message)),
    events: d.events.filter((e) => hit(e.type, e.reason, e.message, e.source)),
    manifest: d.manifest
      .split('\n')
      .map((text, i) => ({ n: i + 1, text }))
      .filter((l) => l.text !== '' && hit(l.text)),
  };
});

const SECTION_IDS = [
  'overview',
  'labels',
  'annotations',
  'owners',
  'conditions',
  'events',
  'manifest',
] as const;
type SectionId = (typeof SECTION_IDS)[number];

const shown = computed(() => {
  const s = sections.value;
  if (!s) return [];
  const filtering = needle.value !== '';
  return SECTION_IDS.filter((id) =>
    filtering ? s[id].length > 0 : id === 'events' || s[id].length > 0,
  );
});
const LABELS: Record<SectionId, () => string> = {
  overview: () => t('docker.tab.overview.label'),
  labels: () => t('docker.field.labels.label'),
  annotations: () => t('kubernetes.describe.section.annotations.label'),
  owners: () => t('kubernetes.describe.section.owners.label'),
  conditions: () => t('kubernetes.describe.section.conditions.label'),
  events: () => t('kubernetes.describe.section.events.label'),
  manifest: () => t('kubernetes.describe.section.manifest.label'),
};
const label = (id: SectionId): string => LABELS[id]();
</script>

<template>
  <QueryState :query="describe" :is-empty="() => false">
    <div class="flex h-full min-h-0 flex-col" data-testid="kube-describe">
      <div class="flex shrink-0 items-center gap-1.5 px-2 py-1">
        <SearchField
          v-model="filter"
          class="flex-1"
          :placeholder="t('kubernetes.describe.filter.placeholder')"
          data-testid="kube-describe-filter"
        />
        <TooltipIconButton
          icon="copy"
          :label="t('kubernetes.describe.copyManifest.label')"
          data-testid="kube-describe-copy"
          @click="copyText(describe.data.value?.manifest ?? '')"
        />
      </div>
      <div class="min-h-0 flex-1 overflow-auto p-2">
        <EmptyState
          v-if="needle !== '' && shown.length === 0"
          data-testid="kube-describe-no-match"
          :title="t('common.state.noMatches.empty')"
        />
        <template v-for="id in shown" :key="id">
          <section class="mb-3" :data-testid="`kube-describe-section-${id}`">
            <SectionHeading :label="label(id)" />
            <KeyValueList
              v-if="id === 'overview' || id === 'labels' || id === 'annotations' || id === 'owners'"
            >
              <template v-for="[ k, v ] in sections![id]" :key="k">
                <dt><HighlightedText :parts="parts(k)" /></dt>
                <dd class="break-all font-data text-kira-sm"
                  ><HighlightedText :parts="parts(v)" /></dd
                >
              </template>
            </KeyValueList>
            <ul v-else-if="id === 'conditions'" class="m-0 list-none p-0 text-kira-md">
              <li
                v-for="c in sections!.conditions"
                :key="c.type"
                class="py-0.5"
                data-testid="kube-condition"
              >
                <span class="font-medium"><HighlightedText :parts="parts(c.type)" /></span>
                <span class="ml-2 text-muted-foreground"
                  ><HighlightedText :parts="parts(c.status)" /></span
                >
                <span v-if="c.reason" class="ml-2 text-muted-foreground"
                  ><HighlightedText :parts="parts(c.reason)" /></span
                >
                <div v-if="c.message" class="text-kira-sm text-muted-foreground"
                  ><HighlightedText :parts="parts(c.message)" /></div
                >
              </li>
            </ul>
            <template v-else-if="id === 'events'">
              <div
                v-if="sections!.events.length === 0"
                class="text-kira-md text-subtle"
                data-testid="kube-describe-no-events"
                >{{
                  t('kubernetes.describe.noEvents.empty')
                }}</div
              >
              <ul v-else class="m-0 list-none p-0 text-kira-md">
                <li
                  v-for="(e, i) in sections!.events"
                  :key="i"
                  class="py-0.5"
                  data-testid="kube-event"
                >
                  <span :class="e.type === 'Warning' ? 'text-warn' : 'text-muted-foreground'"
                    ><HighlightedText :parts="parts(e.type)" /></span
                  >
                  <span class="ml-2 font-medium"><HighlightedText :parts="parts(e.reason)" /></span>
                  <span v-if="e.source" class="ml-2 text-muted-foreground"
                    ><HighlightedText :parts="parts(e.source)" /></span
                  >
                  <span v-if="e.count > 1" class="ml-2 text-muted-foreground">×{{ e.count }}</span>
                  <div class="text-kira-sm text-muted-foreground"
                    ><HighlightedText :parts="parts(e.message)" /></div
                  >
                </li>
              </ul>
            </template>
            <pre
              v-else
              class="m-0 overflow-auto font-data text-kira-sm leading-relaxed"
              data-testid="kube-manifest"
            ><template v-for="l in sections!.manifest" :key="l.n"><span class="mr-3 inline-block w-8 select-none text-right text-subtle">{{ l.n }}</span><HighlightedText :parts="parts(l.text)" />
</template></pre>
          </section>
        </template>
      </div>
    </div>
  </QueryState>
</template>
