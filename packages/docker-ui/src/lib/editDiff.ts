import { type MessageKey, t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import type {
  DockerEditNetwork,
  DockerInPlaceSpec,
  DockerRecreateSpec,
  DockerResources,
} from '../wire';

export type EditMode = 'now' | 'recreate';

export interface EditValues {
  inPlace: DockerInPlaceSpec;
  recreate: DockerRecreateSpec;
}

export interface PendingChange {
  section: 'inPlace' | 'recreate';
  field: string;
  label: string;
  from: string;
  to: string;
  mode: EditMode;
}

type Unsectioned = Omit<PendingChange, 'section'>;

const MIB = 1024 * 1024;
const MIN_MEMORY = 6 * MIB;
const NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9_.-]+$/;
const CPUSET_RE = /^\d+(-\d+)?(,\d+(-\d+)?)*$/;

export function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (typeof a !== 'object' || typeof b !== 'object' || a === null || b === null) return false;
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  const ka = Object.keys(a);
  if (ka.length !== Object.keys(b).length) return false;
  return ka.every((k) =>
    deepEqual((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k]),
  );
}

export function cloneValues<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

/**
 * Three-way merge of plain JSON: a field the draft left alone takes the new base, a field the user
 * edited keeps the draft. Arrays are leaves.
 */
export function merge3<T>(oldBase: T, draft: T, newBase: T): T {
  const isObj = (v: unknown): v is Record<string, unknown> =>
    typeof v === 'object' && v !== null && !Array.isArray(v);
  if (deepEqual(draft, oldBase)) return cloneValues(newBase);
  if (!isObj(draft) || !isObj(oldBase) || !isObj(newBase)) return draft;
  const out: Record<string, unknown> = {};
  for (const k of Object.keys(draft)) out[k] = merge3(oldBase[k], draft[k], newBase[k]);
  return out as T;
}

const SET_FIELDS = [
  'cpuShares',
  'cpusetCpus',
  'cpusetMems',
  'memory',
  'memoryReservation',
  'memorySwap',
  'blkioWeight',
] as const;

const hasCpuQuota = (r: DockerResources): boolean => r.cpuQuota !== 0 || r.cpuPeriod !== 0;

/** True when the engine cannot apply the change to `key` in place (mirrors Go's validateClears). */
export function resourceMode(
  base: DockerResources,
  draft: DockerResources,
  key: keyof DockerResources,
): EditMode {
  if ((SET_FIELDS as readonly string[]).includes(key)) {
    const had = base[key] !== 0 && base[key] !== '';
    const now = draft[key] === 0 || draft[key] === '';
    return had && now ? 'recreate' : 'now';
  }
  if (key === 'pidsLimit') return 'now';
  const cleared = draft.nanoCpus === 0 && !hasCpuQuota(draft);
  const switched =
    (base.nanoCpus !== 0 && hasCpuQuota(draft)) || (hasCpuQuota(base) && draft.nanoCpus !== 0);
  const hadCpu = base.nanoCpus !== 0 || hasCpuQuota(base);
  return switched || (hadCpu && cleared) ? 'recreate' : 'now';
}

const RES_LABEL: Record<keyof DockerResources, MessageKey> = {
  cpuShares: 'docker.edit.cpuShares.label',
  nanoCpus: 'docker.edit.cpus.label',
  cpuQuota: 'docker.edit.cpuQuota.label',
  cpuPeriod: 'docker.edit.cpuPeriod.label',
  cpusetCpus: 'docker.edit.cpuset.label',
  cpusetMems: 'docker.edit.cpusetMems.label',
  memory: 'common.module.memory.label',
  memorySwap: 'docker.edit.swap.label',
  memoryReservation: 'docker.edit.memoryReservation.label',
  blkioWeight: 'docker.edit.blkio.label',
  pidsLimit: 'docker.edit.pids.label',
};

const SIZE_KEYS = new Set<keyof DockerResources>(['memory', 'memorySwap', 'memoryReservation']);

function showResource(key: keyof DockerResources, v: number | string): string {
  if (v === 0 || v === '')
    return key === 'memorySwap'
      ? t('docker.edit.engineDefault.status')
      : t('common.state.none.option');
  if (typeof v === 'string') return v;
  if (key === 'memorySwap' && v === -1) return t('docker.edit.unlimited.label');
  if (SIZE_KEYS.has(key)) return formatBytes(v);
  if (key === 'nanoCpus') return `${v / 1e9}`;
  return String(v);
}

const restartText = (r: DockerInPlaceSpec['restart']): string =>
  r.name === 'on-failure' && r.maxRetries > 0 ? `${r.name}:${r.maxRetries}` : r.name;

function networkChanges(base: DockerEditNetwork[], draft: DockerEditNetwork[]): Unsectioned[] {
  const out: Unsectioned[] = [];
  const b = new Map(base.map((n) => [n.name, n]));
  const d = new Map(draft.map((n) => [n.name, n]));
  const text = (n?: DockerEditNetwork): string =>
    n ? n.aliases.join(', ') || t('docker.edit.attached.status') : t('docker.edit.detached.status');
  for (const name of new Set([...b.keys(), ...d.keys()])) {
    const from = b.get(name);
    const to = d.get(name);
    if (!deepEqual(from?.aliases, to?.aliases) || !from !== !to) {
      out.push({
        field: `networks.${name}`,
        label: t('docker.edit.network.label', { name }),
        from: text(from),
        to: text(to),
        mode: 'now',
      });
    }
  }
  return out;
}

function mapChanges(
  prefix: string,
  label: (key: string) => string,
  base: Record<string, string>,
  draft: Record<string, string>,
): Unsectioned[] {
  const out: Unsectioned[] = [];
  for (const k of new Set([...Object.keys(base), ...Object.keys(draft)])) {
    if (base[k] !== draft[k]) {
      out.push({
        field: `${prefix}.${k}`,
        label: label(k),
        from: base[k] ?? t('common.unset.status'),
        to: draft[k] ?? t('common.removed.status'),
        mode: 'recreate',
      });
    }
  }
  return out;
}

function envMap(env: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const e of env) {
    const i = e.indexOf('=');
    out[i === -1 ? e : e.slice(0, i)] = i === -1 ? '' : e.slice(i + 1);
  }
  return out;
}

const portText = (p: DockerRecreateSpec['ports'][number]): string =>
  `${p.hostIp ? `${p.hostIp}:` : ''}${p.hostPort || t('docker.endpoint.automatic.option')} -> ${p.containerPort}/${p.proto}`;

const mountText = (m: DockerRecreateSpec['mounts'][number]): string =>
  `${m.source} -> ${m.target}${m.readOnly ? ` (${t('docker.edit.readOnlyShort.example')})` : ''}`;

function setChanges(field: string, label: string, base: string[], draft: string[]): Unsectioned[] {
  const out: Unsectioned[] = [];
  for (const x of base) {
    if (!draft.includes(x))
      out.push({ field, label, from: x, to: t('common.removed.status'), mode: 'recreate' });
  }
  for (const x of draft) {
    if (!base.includes(x))
      out.push({ field, label, from: t('common.state.none.option'), to: x, mode: 'recreate' });
  }
  return out;
}

function mountChanges(
  base: DockerRecreateSpec['mounts'],
  draft: DockerRecreateSpec['mounts'],
): Unsectioned[] {
  const out: Unsectioned[] = [];
  const kept = new Set(draft.filter((m) => m.key).map((m) => m.key));
  for (const m of base) {
    if (!kept.has(m.key))
      out.push({
        field: 'mounts',
        label: t('docker.edit.mount.label'),
        from: mountText(m),
        to: t('common.removed.status'),
        mode: 'recreate',
      });
  }
  for (const m of draft) {
    const was = base.find((b) => b.key === m.key);
    if (!m.key)
      out.push({
        field: 'mounts',
        label: t('docker.edit.mount.label'),
        from: t('common.state.none.option'),
        to: mountText(m),
        mode: 'recreate',
      });
    else if (was && !deepEqual(was, m)) {
      out.push({
        field: 'mounts',
        label: t('docker.edit.mount.label'),
        from: mountText(was),
        to: mountText(m),
        mode: 'recreate',
      });
    }
  }
  return out;
}

function inPlaceChanges(base: DockerInPlaceSpec, draft: DockerInPlaceSpec): Unsectioned[] {
  const out: Unsectioned[] = [];
  if (base.name !== draft.name) {
    out.push({
      field: 'name',
      label: t('common.field.name.label'),
      from: base.name,
      to: draft.name,
      mode: 'now',
    });
  }
  for (const key of Object.keys(RES_LABEL) as Array<keyof DockerResources>) {
    if (base.resources[key] !== draft.resources[key]) {
      out.push({
        field: `resources.${key}`,
        label: t(RES_LABEL[key]),
        from: showResource(key, base.resources[key]),
        to: showResource(key, draft.resources[key]),
        mode: resourceMode(base.resources, draft.resources, key),
      });
    }
  }
  if (!deepEqual(base.restart, draft.restart)) {
    out.push({
      field: 'restart',
      label: t('docker.field.restartPolicy.label'),
      from: restartText(base.restart),
      to: restartText(draft.restart),
      mode: 'now',
    });
  }
  return [...out, ...networkChanges(base.networks, draft.networks)];
}

function recreateChanges(base: DockerRecreateSpec, draft: DockerRecreateSpec): Unsectioned[] {
  const out: Unsectioned[] = [];
  const scalar = (field: string, label: string, from: string, to: string): void => {
    if (from !== to)
      out.push({
        field,
        label,
        from: from || t('common.state.none.option'),
        to: to || t('common.state.none.option'),
        mode: 'recreate',
      });
  };
  scalar('image', t('docker.field.image.label'), base.image, draft.image);
  scalar(
    'entrypoint',
    t('docker.field.entrypoint.label'),
    base.entrypoint.join(' '),
    draft.entrypoint.join(' '),
  );
  scalar('cmd', t('common.field.command.label'), base.cmd.join(' '), draft.cmd.join(' '));
  scalar('user', t('common.user.label'), base.user, draft.user);
  scalar('hostname', t('docker.edit.hostname.label'), base.hostname, draft.hostname);
  out.push(
    ...mapChanges(
      'env',
      (k) => t('docker.edit.variable.label', { key: k }),
      envMap(base.env),
      envMap(draft.env),
    ),
  );
  out.push(
    ...mapChanges(
      'labels',
      (k) => t('docker.resource.labelKey.label', { key: k }),
      base.labels,
      draft.labels,
    ),
  );
  out.push(
    ...setChanges(
      'ports',
      t('common.port.label'),
      base.ports.map(portText),
      draft.ports.map(portText),
    ),
  );
  out.push(...mountChanges(base.mounts, draft.mounts));
  out.push(...setChanges('capAdd', t('docker.edit.addCap.label'), base.capAdd, draft.capAdd));
  out.push(...setChanges('capDrop', t('docker.edit.dropCap.label'), base.capDrop, draft.capDrop));
  return out;
}

export function changes(base: EditValues, draft: EditValues): PendingChange[] {
  const tag =
    (section: PendingChange['section']) =>
    (c: Unsectioned): PendingChange => ({ section, ...c });
  return [
    ...inPlaceChanges(base.inPlace, draft.inPlace).map(tag('inPlace')),
    ...recreateChanges(base.recreate, draft.recreate).map(tag('recreate')),
  ];
}

/** The in-place section as the engine can take it: edits that need a recreate revert to the base. */
export function inPlaceForApply(
  base: DockerInPlaceSpec,
  draft: DockerInPlaceSpec,
): DockerInPlaceSpec {
  const out = cloneValues(draft);
  const res = out.resources as unknown as Record<string, number | string>;
  for (const key of Object.keys(RES_LABEL) as Array<keyof DockerResources>) {
    if (resourceMode(base.resources, draft.resources, key) === 'recreate') {
      res[key] = base.resources[key];
    }
  }
  return out;
}

export function hasNowChanges(base: EditValues, draft: EditValues): boolean {
  return !deepEqual(inPlaceForApply(base.inPlace, draft.inPlace), base.inPlace);
}

type Errors = Record<string, string>;

function resourceErrors(r: DockerResources, e: Errors): void {
  if (r.memory !== 0 && r.memory < MIN_MEMORY)
    e['resources.memory'] = t('docker.edit.error.memoryMin.error', {
      min: `6 ${t('docker.unit.mib.option')}`,
    });
  if (r.memory !== 0 && r.memoryReservation > r.memory) {
    e['resources.memoryReservation'] = t('docker.edit.error.reservationMax.error');
  }
  if (r.memorySwap < -1 || (r.memorySwap > 0 && r.memorySwap < r.memory)) {
    e['resources.memorySwap'] = t('docker.edit.error.swapMin.error');
  }
  if (r.cpuShares !== 0 && r.cpuShares < 2)
    e['resources.cpuShares'] = t('docker.edit.error.atLeast.error', { min: 2 });
  if (r.blkioWeight !== 0 && (r.blkioWeight < 10 || r.blkioWeight > 1000)) {
    e['resources.blkioWeight'] = t('docker.edit.error.range.error', { min: 10, max: 1000 });
  }
  if (r.pidsLimit < 0) e['resources.pidsLimit'] = t('docker.edit.error.negative.error');
}

function cpuErrors(r: DockerResources, e: Errors): void {
  if (r.nanoCpus < 0) e['resources.nanoCpus'] = t('docker.edit.error.negative.error');
  if (r.nanoCpus !== 0 && hasCpuQuota(r))
    e['resources.nanoCpus'] = t('docker.edit.error.cpusQuota.error');
  if (r.cpuQuota !== 0 && r.cpuQuota < 1000)
    e['resources.cpuQuota'] = t('docker.edit.error.atLeast.error', { min: 1000 });
  if (r.cpuPeriod !== 0 && (r.cpuPeriod < 1000 || r.cpuPeriod > 1_000_000)) {
    e['resources.cpuPeriod'] = t('docker.edit.error.range.error', { min: 1000, max: 1_000_000 });
  }
  if (r.cpusetCpus !== '' && !CPUSET_RE.test(r.cpusetCpus))
    e['resources.cpusetCpus'] = t('docker.edit.error.cpusetForm.error');
  if (r.cpusetMems !== '' && !CPUSET_RE.test(r.cpusetMems))
    e['resources.cpusetMems'] = t('docker.edit.error.cpusetForm.error');
}

function portErrors(ports: DockerRecreateSpec['ports'], e: Errors): void {
  const seen = new Set<string>();
  for (const p of ports) {
    const key = portText(p);
    if (p.containerPort < 1 || p.containerPort > 65535)
      e['recreate.ports'] = t('docker.edit.error.containerPort.error');
    else if (p.hostPort !== '' && !/^\d{1,5}$/.test(p.hostPort))
      e['recreate.ports'] = t('docker.edit.error.hostPort.error');
    else if (seen.has(key))
      e['recreate.ports'] = t('docker.edit.error.duplicatePort.error', { port: key });
    seen.add(key);
  }
}

function mountErrors(mounts: DockerRecreateSpec['mounts'], e: Errors): void {
  const targets = new Set<string>();
  for (const m of mounts) {
    if (!m.target.startsWith('/'))
      e['recreate.mounts'] = t('docker.edit.error.targetAbsolute.error');
    else if (targets.has(m.target))
      e['recreate.mounts'] = t('docker.edit.error.duplicateTarget.error', { target: m.target });
    else if (m.source === '') e['recreate.mounts'] = t('docker.edit.error.sourceRequired.error');
    targets.add(m.target);
  }
}

/** Client-side mirror of Go's validation, keyed by the field paths Go reports. Go stays the authority. */
export function fieldErrors(draft: EditValues): Record<string, string> {
  const e: Errors = {};
  const { inPlace: ip, recreate: rc } = draft;
  if (!NAME_RE.test(ip.name)) e.name = t('docker.edit.error.nameForm.error');
  resourceErrors(ip.resources, e);
  cpuErrors(ip.resources, e);
  if (
    ip.restart.maxRetries < 0 ||
    (ip.restart.name !== 'on-failure' && ip.restart.maxRetries !== 0)
  ) {
    e.restart = t('docker.edit.error.maxRetries.error');
  }
  if (rc.image.trim() === '') e['recreate.image'] = t('docker.edit.error.imageRequired.error');
  portErrors(rc.ports, e);
  mountErrors(rc.mounts, e);
  if (rc.env.some((x) => x.indexOf('=') < 1))
    e['recreate.env'] = t('docker.edit.error.envKey.error');
  return e;
}

export function toNumber(v: string | number): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}
