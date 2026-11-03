/**
 * The graph data plane for this page: one lazily opened `gitgraph` Wails stream carrying every
 * `graph.stream` of the window (Go side `internal/graphplane`). A closed stream drops the plane;
 * the next call dials a fresh one.
 */
import {
  createGraphPlane,
  createStreamChannel,
  type GraphPlane,
  type StreamChunkOf,
  type StreamParamsOf,
} from '@kira/git-ipc';
import { windowKey } from '@workbench/bridge/rpc';
// tsconfig.json maps this specifier onto @wailsio/runtime's real types (see bridge/port.ts's own
// identical comment) and resolves it cleanly under typecheck:web; tests/unit's own project has no
// such mapping (breaks Bun's mock.module interception for every transitive importer), so this
// stays the suppress-if-present directive that file already established, not the require-an-error
// kind (which would itself fail where this resolves fine).
// biome-ignore lint/suspicious/noTsIgnore: an "unused directive" kind fails where this resolves fine (see comment above)
// @ts-ignore
import { Stream } from '/wails/runtime.js';
import { gitWindowReady } from './boundGit';

let plane: GraphPlane | undefined;

function dial(): GraphPlane {
  const channel = createStreamChannel(Stream('gitgraph'));
  const next = createGraphPlane(channel);
  channel.onClose(() => {
    if (plane === next) plane = undefined;
  });
  plane = next;
  return next;
}

/** Streams the graph rows of `params.repoId` on this window's connection, which must hold the
 *  repo. Waits for the window reset like every bound call. */
export async function streamGraph(
  params: StreamParamsOf<'graph.stream'>,
  onChunk: (chunk: StreamChunkOf<'graph.stream'>) => void | Promise<void>,
  signal?: AbortSignal,
): Promise<void> {
  await gitWindowReady();
  return (plane ?? dial()).stream(windowKey, params, onChunk, signal);
}
