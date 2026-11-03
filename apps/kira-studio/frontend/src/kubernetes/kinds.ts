import { type MessageKey, t } from '@theme/i18n';
import { compareNatural } from '@workbench/behaviour/match';
import type { KindCategory, KubeKind } from './wire';

export const POD_KIND = 'v1/pods';

export const CATEGORY_ORDER: readonly KindCategory[] = [
  'workloads',
  'network',
  'config',
  'storage',
  'access',
  'cluster',
  'custom',
];

const CATEGORY_LABEL: Record<KindCategory, MessageKey> = {
  workloads: 'kubernetes.kind.category.workloads.label',
  network: 'docker.resource.network.label',
  config: 'kubernetes.kind.category.config.label',
  storage: 'kubernetes.kind.category.storage.label',
  access: 'kubernetes.kind.category.access.label',
  cluster: 'kubernetes.kind.category.cluster.label',
  custom: 'kubernetes.kind.category.custom.label',
};

export const categoryLabel = (c: KindCategory): string => t(CATEGORY_LABEL[c]);

const CATEGORY_ICON: Record<KindCategory, string> = {
  workloads: 'server-process',
  network: 'globe',
  config: 'settings',
  storage: 'database',
  access: 'shield',
  cluster: 'type-hierarchy',
  custom: 'extensions',
};

export const kindIcon = (k: KubeKind): string =>
  k.key === POD_KIND ? 'package' : CATEGORY_ICON[k.category];

/** Pods when listed, else the first kind by category then name. */
export function defaultKind(kinds: readonly KubeKind[]): KubeKind | undefined {
  const pods = kinds.find((k) => k.key === POD_KIND);
  if (pods) return pods;
  return [...kinds].sort(
    (a, b) =>
      CATEGORY_ORDER.indexOf(a.category) - CATEGORY_ORDER.indexOf(b.category) ||
      compareNatural(a.kind, b.kind),
  )[0];
}
