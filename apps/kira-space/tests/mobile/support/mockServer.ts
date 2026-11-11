import { readFileSync } from 'node:fs';
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import { extname, join, resolve } from 'node:path';

// A stand-in for the Go mobileweb server: the built phone app from frontend/dist-mobile plus a
// scripted /api. Served by one process so the SSE stream and the pairing
// long-poll all run for real. State is per worker and reset before each test.

const DIST = resolve(__dirname, '../../../frontend/dist-mobile');
const FIXTURES = resolve(__dirname, '../../fixtures/ade-v2');

const MIME: Readonly<Record<string, string>> = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.ico': 'image/x-icon',
  '.ttf': 'font/ttf',
};

const AGENT_INFO = JSON.parse(
  readFileSync(resolve(__dirname, '../../contract/agent.json'), 'utf8'),
) as Record<string, unknown>;

const fixture = (name: string): unknown =>
  JSON.parse(readFileSync(join(FIXTURES, `${name}.json`), 'utf8'));

export type PairOutcome = 'approve' | 'deny' | 'timeout';

interface State {
  /** `/api/me` and every device route answer 200, 401 unauthorized, revoked or expired. */
  auth: 'ok' | 'unauthorized' | 'revoked' | 'expired';
  /** What `/api/me` reports the desktop allows this phone. */
  permissions: { write: boolean; agentInput: boolean };
  agentInputGlobal: boolean;
  requests: { method: string; path: string; body: string; key: string }[];
  /** One-shot scripted failure for the next POST to a path. */
  failNext: Map<string, { status: number; code: string; message: string }>;
  /** What `/api/agent/sessions` lists: the live Claude Code tabs. */
  agentSessions: { terminalId: string; cwd: string }[];
  /** Backend-shaped bodies a spec reads from a contract fixture; unset keeps the canned answer. */
  replies: {
    me?: unknown;
    pair?: unknown;
    expired?: unknown;
    board?: unknown;
    laterTask?: unknown;
    sessions?: unknown;
  };
  pairWaiters: ServerResponse[];
  /** Outcome resolved before the phone's `POST /api/pair` arrived; the request answers on arrival. */
  earlyPair: PairOutcome | null;
  streams: Set<ServerResponse>;
}

export interface MobileServer {
  url: string;
  state: State;
  reset(): void;
  /** Sends one server-sent event to every open stream. */
  emit(channel: string, data: unknown): void;
  /** Answers every parked `POST /api/pair`; approving also marks the device paired. */
  resolvePair(outcome: PairOutcome): void;
  /** Requests that changed state, i.e. anything but GET. */
  nonGet(): { method: string; path: string }[];
  close(): Promise<void>;
}

function json(res: ServerResponse, status: number, body: unknown): void {
  res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
  res.end(JSON.stringify(body));
}

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((done) => {
    let data = '';
    req.on('data', (c) => {
      data += c;
    });
    req.on('end', () => done(data));
  });
}

async function serveFile(res: ServerResponse, pathname: string): Promise<void> {
  const rel = pathname === '/' ? 'index.html' : pathname.replace(/^\/+/, '');
  const file = join(DIST, rel);
  if (!file.startsWith(DIST)) {
    res.writeHead(403).end();
    return;
  }
  try {
    const body = readFileSync(file);
    const noCache = rel === 'sw.js' || rel.endsWith('.html');
    res.writeHead(200, {
      'Content-Type': MIME[extname(file)] ?? 'application/octet-stream',
      'Cache-Control': noCache ? 'no-cache' : 'public, max-age=31536000',
    });
    res.end(body);
  } catch {
    // A client-side route: any extensionless navigation gets the app shell.
    if (extname(rel) === '') await serveFile(res, '/');
    else res.writeHead(404).end();
  }
}

// Canned answers for the phone's write routes; undefined means the path is not a write route.
function writeReply(path: string, body: string): unknown {
  const args = body ? (JSON.parse(body) as Record<string, string>) : {};
  if (path === '/api/ade/later/items') return { id: 't-new', title: args.title, notes: '' };
  if (path === '/api/ade/shelf/move') return {};
  if (path === '/api/ade/tasks/step') return { taskId: args.taskId, stepId: args.stepId };
  if (path === '/api/ade/tasks/run') return { runIds: ['run-new'] };
  if (path === '/api/ade/tasks/launch') return { sessionId: 'sp22' };
  const session = /^\/api\/agent\/sessions\/([^/]+)\/(send|take-over)$/.exec(path);
  if (session) return session[2] === 'send' ? {} : { sessionId: session[1] };
  return undefined;
}

export async function startMobileServer(): Promise<MobileServer> {
  const initial = (): State => ({
    auth: 'unauthorized',
    permissions: { write: false, agentInput: false },
    agentInputGlobal: false,
    requests: [],
    failNext: new Map(),
    agentSessions: [],
    replies: {},
    pairWaiters: [],
    earlyPair: null,
    streams: new Set(),
  });
  const state: State = initial();

  function answerPair(outcome: PairOutcome): void {
    for (const res of state.pairWaiters) {
      if (outcome === 'approve') {
        state.auth = 'ok';
        json(res, 200, state.replies.pair ?? { deviceId: 'dev-1', label: 'Test phone' });
      } else {
        json(res, 403, {
          code: 'E_PAIRING_DENIED',
          message: outcome === 'deny' ? 'access denied' : 'request timed out',
          reason: outcome === 'deny' ? 'denied' : 'timeout',
        });
      }
    }
    state.pairWaiters = [];
  }
  const repos = (
    fixture('repos') as { repos: { codeRepoId: string; name: string; nickname: string }[] }
  ).repos.map(({ codeRepoId, name, nickname }) => ({ codeRepoId, name, nickname }));
  const reads: Record<string, unknown> = {
    '/api/ade/board': fixture('board'),
    '/api/ade/change-requests': fixture('change-requests'),
    '/api/ade/sessions': fixture('sessions'),
    '/api/ade/workflows': fixture('workflows'),
    '/api/ade/repos': { repos },
    '/api/ade/hosts': fixture('host-providers'),
    '/api/ade/log': fixture('log-page'),
  };

  const unauthorized = (res: ServerResponse): void => {
    const failure = {
      revoked: { code: 'E_REVOKED', message: 'device revoked' },
      expired: state.replies.expired ?? { code: 'E_EXPIRED', message: 'device access expired' },
    }[state.auth as 'revoked' | 'expired'] ?? { code: 'E_UNAUTHORIZED', message: 'not paired' };
    json(res, 401, failure);
  };

  const handle = async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    const url = new URL(req.url ?? '/', 'http://localhost');
    const path = url.pathname;
    const method = req.method ?? 'GET';
    if (!path.startsWith('/api/')) {
      await serveFile(res, path);
      return;
    }
    const body = method === 'GET' ? '' : await readBody(req);
    state.requests.push({ method, path, body, key: String(req.headers['idempotency-key'] ?? '') });

    if (path === '/api/pair' && method === 'POST') {
      state.pairWaiters.push(res);
      res.on('close', () => {
        state.pairWaiters = state.pairWaiters.filter((r) => r !== res);
      });
      // The pairing screen shows "waiting" before the POST reaches the server, so a test can
      // resolve first.
      if (state.earlyPair) {
        const outcome = state.earlyPair;
        state.earlyPair = null;
        answerPair(outcome);
      }
      return;
    }
    if (state.auth !== 'ok') {
      unauthorized(res);
      return;
    }
    if (path === '/api/me') {
      json(
        res,
        200,
        state.replies.me ?? {
          deviceId: 'dev-1',
          label: 'Test phone',
          permissions: state.permissions,
          agentInputGlobal: state.agentInputGlobal,
        },
      );
    } else if (path === '/api/events') {
      res.writeHead(200, {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-store',
        Connection: 'keep-alive',
      });
      res.write(': open\n\n');
      state.streams.add(res);
      res.on('close', () => state.streams.delete(res));
    } else if (method === 'POST' && writeReply(path, body) !== undefined) {
      const fail = state.failNext.get(path);
      if (fail) {
        state.failNext.delete(path);
        json(res, fail.status, { code: fail.code, message: fail.message });
      } else {
        json(
          res,
          200,
          path === '/api/ade/later/items' && state.replies.laterTask
            ? state.replies.laterTask
            : writeReply(path, body),
        );
      }
    } else if (path === '/api/agent/info' && method === 'GET') {
      json(res, 200, AGENT_INFO['AgentService.Info']);
    } else if (path === '/api/agent/sessions' && method === 'GET') {
      json(res, 200, { sessions: state.agentSessions });
    } else if (path === '/api/ade/sessions' && method === 'GET' && state.replies.sessions) {
      json(res, 200, state.replies.sessions);
    } else if (path === '/api/ade/board' && method === 'GET' && state.replies.board) {
      json(res, 200, state.replies.board);
    } else if (path in reads && method === 'GET') {
      json(res, 200, reads[path]);
    } else {
      json(res, 404, { code: 'E_NOT_FOUND', message: 'not found' });
    }
  };

  const server: Server = createServer((req, res) => void handle(req, res));
  await new Promise<void>((ok, fail) => {
    server.once('error', fail);
    server.listen(0, '127.0.0.1', () => ok());
  });
  const address = server.address();
  const port = typeof address === 'object' && address ? address.port : 0;

  const api: MobileServer = {
    url: `http://127.0.0.1:${port}/`,
    state,
    reset() {
      for (const res of [...state.streams, ...state.pairWaiters]) res.destroy();
      Object.assign(state, initial());
    },
    emit(channel, data) {
      for (const res of state.streams)
        res.write(`event: ${channel}\ndata: ${JSON.stringify(data)}\n\n`);
    },
    resolvePair(outcome) {
      if (state.pairWaiters.length === 0) state.earlyPair = outcome;
      else answerPair(outcome);
    },
    nonGet: () =>
      state.requests
        .filter((r) => r.method !== 'GET')
        .map(({ method, path }) => ({ method, path })),
    close: () =>
      new Promise<void>((ok, fail) => {
        api.reset();
        server.closeAllConnections();
        server.close((err) => (err ? fail(err) : ok()));
      }),
  };
  return api;
}
