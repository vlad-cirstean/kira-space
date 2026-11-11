import { type MessageKey, t } from '@theme/i18n';

const SOURCE_LABEL: Record<string, MessageKey> = {
  task: 'common.task.label',
  host: 'common.gitHost.label',
  jira: 'common.product.jira.label',
  git: 'common.module.git.label',
  agent: 'memory.common.origin.agent.label',
  user: 'common.picker.you.label',
};

/** An artifact source in words; an unknown source shows as sent. */
export function sourceLabel(source: string): string {
  const key = SOURCE_LABEL[source];
  return key ? t(key) : source;
}
