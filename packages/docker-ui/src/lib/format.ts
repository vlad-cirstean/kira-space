import type { StatusTone } from '@theme/lib/statusTone';
import type { ContainerState, DockerPort } from '../wire';

export function shortId(id: string): string {
  return id.replace(/^sha256:/, '').slice(0, 12);
}

/** A resource's name, or its short id while the name is unknown. */
export const nameOrShortId = (name: string | undefined, id: string): string => name ?? shortId(id);

export function isActiveState(state: ContainerState): boolean {
  return state === 'running' || state === 'paused' || state === 'restarting';
}

/** RS.1 tone for a container state. */
export function containerStateTone(state: ContainerState): StatusTone {
  if (state === 'running') return 'ok';
  if (state === 'paused' || state === 'restarting') return 'warn';
  if (state === 'dead') return 'error';
  return 'neutral';
}

/** Tailwind background for a usage bar: calm until it nears the limit. */
export function usageToneClass(percent: number): string {
  if (percent >= 90) return 'bg-error';
  if (percent >= 70) return 'bg-warn';
  return 'bg-info';
}

/** Published host ports, deduplicated (IPv4 and IPv6 bindings repeat), in port order. */
export function publishedPorts(ports: readonly DockerPort[]): string[] {
  const seen = new Set<number>();
  for (const p of ports) if (p.publicPort > 0) seen.add(p.publicPort);
  return [...seen].sort((a, b) => a - b).map(String);
}
