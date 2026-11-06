import { describe, expect, spyOn, test } from 'bun:test';
import {
  createParseClient,
  type WorkerLike,
} from '../../../frontend/src/studio/workers/parse/client';
import { handlers } from '../../../frontend/src/studio/workers/parse/handlers';
import type {
  WorkerRequest,
  WorkerResponse,
} from '../../../frontend/src/studio/workers/parse/protocol';

class FakeWorker implements WorkerLike {
  static all: FakeWorker[] = [];
  posted: WorkerRequest[] = [];
  terminated = false;
  private onMessage: Array<(e: MessageEvent<WorkerResponse>) => void> = [];
  private onError: Array<(e: unknown) => void> = [];
  constructor() {
    FakeWorker.all.push(this);
  }
  postMessage(m: WorkerRequest): void {
    this.posted.push(m);
  }
  terminate(): void {
    this.terminated = true;
  }
  addEventListener(type: 'message', cb: (e: MessageEvent<WorkerResponse>) => void): void;
  addEventListener(type: 'error', cb: (e: unknown) => void): void;
  addEventListener(type: string, cb: (e: never) => void): void {
    if (type === 'message') this.onMessage.push(cb as never);
    else this.onError.push(cb as never);
  }
  reply(r: WorkerResponse): void {
    for (const cb of this.onMessage) cb({ data: r } as MessageEvent<WorkerResponse>);
  }
  fail(): void {
    for (const cb of this.onError) cb(new Error('boom'));
  }
}

const tick = () => new Promise<void>((r) => setTimeout(r, 0));
const input = { text: '{"a":1}', mode: 'indented' as const };

function setup() {
  FakeWorker.all = [];
  return createParseClient(() => new FakeWorker());
}

describe('parse worker client', () => {
  test('posts one job at a time, FIFO', async () => {
    const c = setup();
    const a = c.run('json.beautify', input);
    const b = c.run('json.beautify', input);
    const w = FakeWorker.all[0] as FakeWorker;
    expect(w.posted.map((m) => m.id)).toEqual([0]);
    w.reply({ id: 0, ok: true, output: 'A' });
    expect(await a).toBe('A' as never);
    expect(w.posted.map((m) => m.id)).toEqual([0, 1]);
    w.reply({ id: 1, ok: true, output: 'B' });
    expect(await b).toBe('B' as never);
  });

  test('aborting a queued job removes it before posting', async () => {
    const c = setup();
    const ctrl = new AbortController();
    const a = c.run('json.beautify', input);
    const b = c.run('json.beautify', input, { signal: ctrl.signal });
    ctrl.abort();
    await expect(b).rejects.toMatchObject({ name: 'AbortError' });
    const w = FakeWorker.all[0] as FakeWorker;
    w.reply({ id: 0, ok: true, output: 'A' });
    await a;
    expect(w.posted.map((m) => m.id)).toEqual([0]);
  });

  test('aborting the running job with nothing waiting lets it finish and drops the result', async () => {
    const c = setup();
    const ctrl = new AbortController();
    const a = c.run('json.beautify', input, { signal: ctrl.signal });
    ctrl.abort();
    await expect(a).rejects.toMatchObject({ name: 'AbortError' });
    expect((FakeWorker.all[0] as FakeWorker).terminated).toBe(false);
  });

  test('a job queued behind an aborted running job terminates and respawns the worker', async () => {
    const c = setup();
    const ctrl = new AbortController();
    const a = c.run('json.beautify', input, { signal: ctrl.signal });
    ctrl.abort();
    await expect(a).rejects.toMatchObject({ name: 'AbortError' });
    const b = c.run('json.beautify', input);
    expect(FakeWorker.all.length).toBe(2);
    expect((FakeWorker.all[0] as FakeWorker).terminated).toBe(true);
    (FakeWorker.all[1] as FakeWorker).reply({ id: 1, ok: true, output: 'B' });
    expect(await b).toBe('B' as never);
  });

  test('a worker error runs in-flight and queued jobs inline, then respawns on the next job', async () => {
    const c = setup();
    const a = c.run('json.beautify', input);
    const b = c.run('json.beautify', input);
    (FakeWorker.all[0] as FakeWorker).fail();
    expect(((await a) as { ok: boolean }).ok).toBe(true);
    expect(((await b) as { ok: boolean }).ok).toBe(true);
    void c.run('json.beautify', input);
    await tick();
    expect(FakeWorker.all.length).toBe(2);
  });

  test('a worker error skips an already-aborted in-flight job', async () => {
    const spy = spyOn(handlers, 'json.beautify');
    try {
      const c = setup();
      const ctrl = new AbortController();
      const a = c.run('json.beautify', input, { signal: ctrl.signal });
      const settled = a.catch((e: unknown) => (e as Error).name);
      ctrl.abort();
      (FakeWorker.all[0] as FakeWorker).fail();
      expect(await settled).toBe('AbortError');
      await tick();
      expect(spy).not.toHaveBeenCalled();
    } finally {
      spy.mockRestore();
    }
  });

  test('a worker that cannot be constructed falls back inline', async () => {
    const c = createParseClient(() => {
      throw new Error('no worker');
    });
    const r = await c.run('json.beautify', input);
    expect(r.ok).toBe(true);
  });

  test('a handler error rejects the job without killing the worker', async () => {
    const c = setup();
    const a = c.run('json.beautify', input);
    const w = FakeWorker.all[0] as FakeWorker;
    w.reply({ id: 0, ok: false, error: 'bad' });
    await expect(a).rejects.toThrow('bad');
    expect(w.terminated).toBe(false);
  });
});
