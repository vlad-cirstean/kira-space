import type { ApiRule } from '../../../state/settingsDomain';

/** The methods the api_request tool accepts, in the order the dialog lists them. */
export const API_METHODS = ['GET', 'HEAD', 'OPTIONS', 'POST', 'PUT', 'PATCH', 'DELETE'] as const;

const LABEL = '[a-z0-9]([a-z0-9-]*[a-z0-9])?';
const NAME = `${LABEL}(\\.${LABEL})*`;
const HOST = new RegExp(`^(\\*\\.)?${NAME}(:\\d{1,5})?$|^\\[[0-9a-f:.]+\\](:\\d{1,5})?$`, 'i');

/**
 * Shape check for one hosts entry: `*`, `host`, `host:port`, `*.domain` or a bracketed IPv6 literal. Go
 * stays authoritative (it also normalizes IDN hosts); this only catches obvious typos before Save.
 */
export function isHostPattern(entry: string): boolean {
  if (entry === '*') return true;
  if (!HOST.test(entry)) return false;
  const port = /:(\d+)$/.exec(entry);
  return !port || (Number(port[1]) >= 1 && Number(port[1]) <= 65535);
}

/** True when rules exist but none allows a request, so every request is refused. */
export function allowsNothing(rules: readonly ApiRule[]): boolean {
  return rules.length > 0 && !rules.some((r) => r.effect === 'allow');
}
