import type {
  ImportFile,
  ImportFileState,
  ImportJob,
  ImportJobState,
} from '@shared/domain/memoryImport';
import type { MessageKey } from '@theme/i18n';

export const ACTIVE_JOB_STATES: readonly ImportJob['state'][] = [
  'scanning',
  'awaiting',
  'running',
  'paused',
];

export const JOB_STATE_LABEL = {
  scanning: 'memory.import.state.scanning.status',
  awaiting: 'memory.import.state.awaiting.status',
  running: 'common.state.running.status',
  paused: 'memory.import.state.paused.status',
  done: 'common.done.status',
  cancelled: 'common.cancelled.status',
  failed: 'common.state.failed.status',
} as const satisfies Record<ImportJobState, MessageKey>;

export const FILE_STATE_LABEL = {
  pending: 'common.state.pending.status',
  skipped: 'common.skipped.status',
  extracting: 'memory.import.fileState.extracting.status',
  extracted: 'memory.import.fileState.extracted.status',
  verifying: 'memory.import.fileState.verifying.status',
  finalizing: 'memory.import.fileState.finalizing.status',
  done: 'common.done.status',
  failed: 'common.state.failed.status',
  cancelled: 'common.cancelled.status',
} as const satisfies Record<ImportFileState, MessageKey>;

export const FILE_KIND_LABEL = {
  markdown: 'memory.import.file.kind.markdown.label',
  text: 'common.field.text.label',
  json: 'common.copyFormat.json.label',
  jsonl: 'memory.import.file.kind.jsonl.label',
  memdoc: 'memory.import.file.kind.memdoc.label',
} as const satisfies Record<ImportFile['kind'], MessageKey>;

export function jobTitle(job: ImportJob): string {
  const first = job.roots[0] ?? job.base;
  const name = first.split(/[\\/]/).filter(Boolean).pop() ?? first;
  return job.roots.length > 1 ? `${name} +${job.roots.length - 1}` : name;
}
