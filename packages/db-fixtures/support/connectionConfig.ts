import type { ConnectionSummary } from '@shared/domain/connection';

// The shape these fixtures hand straight to an adapter's Connect(), test-owned now (it used to be
// packages/shared/protocol/engine-ops.ts's `ResolvedConnectionConfig`, the old main→engine-child
// wire type from the Node-engine-sidecar architecture — nothing under apps/kira-studio/frontend/src
// or the Go adapters package needs this shape today, only these fixtures constructing one to pass
// in-process). `preconnect`/`preconnectSidecar`/`preconnectDelaySec` are omitted the same way the
// original type did: they govern main's own arm()/monitor decision, not anything the adapter itself
// reads.
export type ResolvedConnectionConfig = Omit<
  ConnectionSummary,
  'file' | 'preconnect' | 'preconnectSidecar' | 'preconnectDelaySec'
> & {
  password: string | null;
};
