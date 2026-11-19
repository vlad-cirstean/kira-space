import type {
  Memory,
  MemoryEvent,
  MemoryGroupKind,
  MemoryKind,
  MemoryOrigin,
  MemoryScope,
} from '@shared/domain/memory';
import { type MessageKey, t } from '@theme/i18n';

export const ORIGIN_LABEL = {
  user: 'common.user.label',
  agent: 'common.origin.agent.label',
  external: 'memory.common.origin.external.label',
} as const satisfies Record<MemoryOrigin, MessageKey>;

/** Only a rule is badged; a fact is the default and stays unmarked. */
export const KIND_LABEL = {
  rule: 'memory.common.kind.rule.label',
} as const satisfies Partial<Record<MemoryKind, MessageKey>>;

export const SCOPE_LABEL = {
  company: 'memory.common.scope.company.label',
  project: 'memory.common.scope.project.label',
  personal: 'memory.common.scope.personal.label',
} as const satisfies Record<MemoryScope, MessageKey>;

export const GROUP_KIND_LABEL = {
  session: 'common.field.session.label',
  request: 'studio.http.raw.requestPending.label',
  task: 'common.task.label',
  document: 'memory.common.groupKind.document.label',
  thread: 'memory.common.groupKind.thread.label',
} as const satisfies Record<MemoryGroupKind, MessageKey>;

/** How a search found a fact. */
export const MATCH_LABEL = {
  keyword: 'memory.panel.keyword.status',
  semantic: 'memory.panel.semantic.status',
  both: 'memory.panel.both.status',
} as const satisfies Record<NonNullable<Memory['match']>, MessageKey>;

/** How far to trust a fact, by origin: user above external above agent. */
export const GRADE_TEXT = {
  user: 'memory.common.grade.user.description',
  external: 'memory.common.grade.external.description',
  agent: 'memory.common.grade.agent.description',
} as const satisfies Record<MemoryOrigin, MessageKey>;

/** Badge variant by origin. */
export const ORIGIN_BADGE = {
  user: 'info',
  external: 'warn',
  agent: 'default',
} as const satisfies Record<MemoryOrigin, 'info' | 'warn' | 'default'>;

/** Origin label; an external origin names its source, such as "External · slack". */
export function originText(origin: MemoryOrigin, sourceTag: string | null): string {
  if (origin === 'external' && sourceTag) {
    return t('memory.common.origin.externalTagged.label', { tag: sourceTag });
  }
  return t(ORIGIN_LABEL[origin]);
}

const ACTION_LABEL = {
  noop: 'memory.start.event.action.reconfirmed.status',
  add: 'common.added.status',
  update: 'memory.start.event.action.updated.status',
} as const satisfies Record<MemoryEvent['action'], MessageKey>;

const SOURCE_LABEL = {
  mcp: 'common.mcp.label',
  ui: 'memory.start.event.source.ui.label',
  import: 'common.import.label',
} as const satisfies Record<MemoryEvent['source'], MessageKey>;

/** One line of a version's trail: what happened, by whom, through which door, and why. */
export function eventText(event: MemoryEvent): string {
  const params = {
    action: t(ACTION_LABEL[event.action]),
    origin: originText(event.origin, event.sourceTag),
    source: t(SOURCE_LABEL[event.source]),
    label: event.sourceLabel ?? event.sourceRef ?? '',
    rationale: event.rationale,
  };
  const labelled = event.source === 'import';
  if (labelled && event.rationale) return t('memory.start.event.withBoth.label', params);
  if (labelled) return t('memory.start.event.withLabel.label', params);
  if (event.rationale) return t('memory.start.event.withRationale.label', params);
  return t('memory.start.event.plain.label', params);
}
