import type { ProofView } from '@shared/domain/proof';

const ICONS: Record<ProofView, string> = {
  table: 'table',
  document: 'json',
  keyvalue: 'symbol-key',
  chart: 'graph',
  diagram: 'type-hierarchy-sub',
};

export function proofViewIcon(view: ProofView): string {
  return ICONS[view];
}
