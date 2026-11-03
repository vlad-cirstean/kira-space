/**
 * Stream-chunk codec: `encodeStreamPayload`/`decodeStreamPayload` turn one stream chunk's
 * buffer-bearing sub-field (today, only `"graph.stream"`'s `commits: PackedCommitChunk`) into a
 * single tagged `ArrayBuffer` (FlatBuffers, P16 W6) and back.
 */
import type { PackedCommitChunk, StreamKey } from './contract.ts';
import { fromWire as graphChunkFromWire, toWire as graphChunkToWire } from './graphChunkCodec.ts';

// ---------------------------------------------------------------------------------------
// The FlatBuffers seam. Keyed on StreamKey, not on the payload's shape: today only
// "graph.stream" carries a schema this way, and the never-defaulted switch below means the next
// stream key added to the contract is a compile error here, not a silent pass-through.
// ---------------------------------------------------------------------------------------

/** The `commits` field, once reduced to a single tagged `ArrayBuffer` by a schema-specific
 *  `toWire`. `$fb` names *which* schema (and, after the slash, which version of this dispatch's
 *  own wrapping — not the FlatBuffers schema's own append-only evolution, which needs no such
 *  tag) produced `d`, so `decodeStreamPayload` can fail loudly on a label it does not
 *  recognise instead of misreading someone else's bytes. */
interface FlatBufferStreamPayload {
  readonly $fb: string;
  readonly d: ArrayBuffer;
}

function isFlatBufferStreamPayload(value: unknown): value is FlatBufferStreamPayload {
  return (
    value !== null &&
    typeof value === 'object' &&
    typeof (value as { $fb?: unknown }).$fb === 'string'
  );
}

/** `graph.stream`'s chunk envelope (`Contract["streams"]["graph.stream"]["chunk"]`), with
 *  `commits` still a plain `PackedCommitChunk` (pre-`encodeStreamPayload`) or already wrapped
 *  (post-`encodeStreamPayload`) — the envelope's seven scalars
 *  (`repoId`, `seq`, `from`, `to`, `source`, `remaining`, `exhausted`) cost ~100 bytes and are not
 *  worth a second schema statement, so only `commits` — the 13 fields that carry
 *  bytes — moves onto FlatBuffers. */
interface GraphStreamEnvelope<TCommits> {
  readonly commits: TCommits;
}

/** Wraps one stream's chunk for the wire. `method` selects the schema-specific `toWire`, applied
 *  only to the sub-field that carries `ArrayBuffer`s — the rest of the envelope crosses
 *  unchanged, exactly as it always has. Every `StreamKey` the contract declares must appear here
 *  (the `never` default is the compile-time guard — the next stream key added to
 *  `Contract["streams"]` and not handled below fails `tsc` here, not at runtime). */
export function encodeStreamPayload(method: StreamKey, chunk: unknown): unknown {
  switch (method) {
    case 'graph.stream': {
      const envelope = chunk as GraphStreamEnvelope<PackedCommitChunk | FlatBufferStreamPayload>;
      // A relay (the extension host) that never decoded this chunk hands it back here already
      // `$fb`-wrapped — toWire+fromWire round tripping bytes that are already exactly what goes out
      // would be pure waste. Recognising the wrapper and returning the chunk unchanged makes this
      // call idempotent for a pass-through caller, without weakening the loud-failure behavior for
      // an unrecognised tag.
      if (isFlatBufferStreamPayload(envelope.commits)) {
        if (envelope.commits.$fb !== 'gitwire/1') {
          throw new Error(
            `codec.encodeStreamPayload: unrecognised '$fb' tag '${envelope.commits.$fb}' for 'graph.stream'`,
          );
        }
        return chunk;
      }
      const wrapped: FlatBufferStreamPayload = {
        $fb: 'gitwire/1',
        d: graphChunkToWire(envelope.commits),
      };
      return { ...envelope, commits: wrapped };
    }
    default: {
      const exhaustive: never = method;
      throw new Error(
        `codec.encodeStreamPayload: unhandled stream key ${JSON.stringify(exhaustive)}`,
      );
    }
  }
}

/** Reverses `encodeStreamPayload`. Throws if `payload.commits` is not a recognised `$fb`-tagged
 *  wrapper — a webview built against a stale contract must fail loudly, not render an empty graph. */
export function decodeStreamPayload(method: StreamKey, payload: unknown): unknown {
  switch (method) {
    case 'graph.stream': {
      const envelope = payload as GraphStreamEnvelope<unknown>;
      const wrapped = envelope.commits;
      if (!isFlatBufferStreamPayload(wrapped)) {
        throw new Error(
          "codec.decodeStreamPayload: 'graph.stream' chunk's 'commits' is missing its '$fb' FlatBuffers tag",
        );
      }
      if (wrapped.$fb !== 'gitwire/1') {
        throw new Error(
          `codec.decodeStreamPayload: unrecognised '$fb' tag '${wrapped.$fb}' for 'graph.stream'`,
        );
      }
      return { ...envelope, commits: graphChunkFromWire(wrapped.d) };
    }
    default: {
      const exhaustive: never = method;
      throw new Error(
        `codec.decodeStreamPayload: unhandled stream key ${JSON.stringify(exhaustive)}`,
      );
    }
  }
}
