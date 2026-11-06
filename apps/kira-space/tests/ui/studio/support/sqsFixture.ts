import { controlSnapshots, portSnapshots } from '../../../ipc/sqs/sqs.fixture';
import type { ControlSnapshot, PortSnapshot } from '../../../ipc/support/types';

export const SQS_QUEUE_PATH = 'queue:orders-queue';
export const SQS_REDRIVE_LIMIT = 3;

interface DefinitionRow {
  name: string;
  value: string;
  detail: string | null;
}

interface DefinitionResponse {
  definition: { sections: { rows: DefinitionRow[] }[] };
}

/** The captured SQS snapshots, patched for a read-only connection: the connection summary is
 *  read-only, the orders queue definition gains a RedrivePolicy row (the capture has none), and
 *  the poll read is re-keyed once per hide time under test. */
export function readOnlySqsSnapshots(visibilityTimeouts: number[]): {
  control: ControlSnapshot[];
  stream: PortSnapshot[];
} {
  const control = structuredClone(controlSnapshots) as ControlSnapshot[];
  for (const snap of control) {
    if (snap.channel === 'kira:connections:list') {
      for (const record of snap.response as { readOnly: boolean }[]) record.readOnly = true;
    }
    if (snap.channel === 'kira:tree:definition') {
      const rows = (snap.response as DefinitionResponse).definition.sections[0].rows;
      rows.push({
        name: 'RedrivePolicy',
        value: JSON.stringify({
          deadLetterTargetArn: 'arn:aws:sqs:us-east-1:000000000000:orders-dlq',
          maxReceiveCount: SQS_REDRIVE_LIMIT,
        }),
        detail: null,
      });
    }
  }

  const stream: PortSnapshot[] = [];
  for (const snap of portSnapshots) {
    if (snap.op !== 'data:read') {
      stream.push(snap);
      continue;
    }
    for (const seconds of visibilityTimeouts) {
      const keyed = structuredClone(snap) as PortSnapshot & { payload: { filter: string | null } };
      keyed.payload.filter = JSON.stringify({ visibilityTimeoutSeconds: seconds });
      stream.push(keyed);
    }
  }
  return { control, stream };
}
