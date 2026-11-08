import type { ProofView } from '@shared/domain/proof';

const ICONS: Record<ProofView, string> = {
  table: 'table',
  document: 'json',
  keyvalue: 'symbol-key',
  chart: 'graph',
  diagram: 'type-hierarchy-sub',
  image: 'file-media',
};

export function proofViewIcon(view: ProofView): string {
  return ICONS[view];
}
