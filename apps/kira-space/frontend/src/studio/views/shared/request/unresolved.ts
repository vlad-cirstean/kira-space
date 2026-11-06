import { isDynamicName, isFakeName } from '@kira/api-core';
import { t } from '@theme/i18n';

interface VariableRef {
  kind: string;
  name: string;
}

/** Refs the request cannot resolve. Catalogued dynamic names (`$name` or `fake.*`) generate at send
 *  time and are never missing. Deduplicated by name. */
export function unresolvedOf<R extends VariableRef>(refs: readonly R[]): R[] {
  const byName = new Map(
    refs
      .filter(
        (r) =>
          r.kind === 'unknown' ||
          (r.kind === 'dynamic' && !isDynamicName(r.name) && !isFakeName(r.name)),
      )
      .map((r) => [r.name, r]),
  );
  return [...byName.values()];
}

export function unresolvedTooltipOf(refs: readonly VariableRef[]): string {
  return refs
    .map((r) =>
      r.kind === 'dynamic'
        ? t('studio.api.common.unknownDynamic.tooltip', { name: r.name })
        : r.name,
    )
    .join(', ');
}
