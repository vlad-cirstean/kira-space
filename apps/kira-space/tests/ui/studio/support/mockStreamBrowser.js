// The browser-side half of tests/ui/support/mockStream.ts's window._wails.streamFactory install
// (P57 D14) — a plain, uncompiled JS file, read as raw text by mockStream.ts and injected via
// `page.addInitScript(text)` rather than `page.addInitScript(fn, arg)`.
//
// P57 finding: passing a function (the natural, typed way) does not work here. Both esbuild-based
// TS loaders this repo's tooling can run under (tsx locally, Playwright's own internal transform)
// compile named functions and object methods under `keepNames`, appending a call like
// `__name(fn, "fn")` after every one of them — a helper defined once at the top of the *whole
// compiled file*, a scope `Function.prototype.toString()` never captures, since `addInitScript`
// serialises only that one function's own source. A same-named local shim does not shadow it
// either: esbuild treats `__name` as a reserved symbol it owns and renames any user declaration
// that collides with it, so the *injected call sites* still reference the (undefined, in the
// extracted text) outer helper regardless. A plain-JS file that is never fed through that
// compiler at all — read with `fs.readFileSync`, not `import` — has no such call sites to begin
// with. `tests/ipc/support/mockPort.ts`'s own `page.evaluate` closures document the same
// self-containment constraint one degree less severely (they run inline, string-injection is a
// TypeScript file's *own* problem, not theirs, since `page.evaluate(fn, arg)` in that file never
// hit this).
//
// P11 C3: responses are FlatBuffers frames now, not JSON. mockStream.ts pre-encodes each one —
// through the real `@shared/protocol/page` builders and the generated wire code — as a
// `{base64, idOffset}` template with `id: 0` and `forceDefaults: true`, so this file only ever
// needs to base64-decode, clone, and patch a real request id in at `idOffset` (a plain int32
// write — no FlatBuffers knowledge belongs here, same as P11 C2 kept page construction out of
// this file for the JSON-envelope wire format it has since replaced). Requests are still JSON
// text (P11 D3) — only responses/events changed.
//
// Wrapped in `(function (init) { ... })` by mockStream.ts, which appends
// `{snapshots, ping, miss}` as a JSON-stringified call argument — everything below is that one
// function's body.
(init) => {
  var CONNECTING = 0;
  var OPEN = 1;
  var CLOSED = 3;

  // Mirrors @wailsio/runtime's stream.js own `defineHandlerProperty`, and
  // @workbench/testing/unit/fakeSocket's copy of it — kept in sync by hand across the three (real
  // runtime, unit fake, this one) rather than shared, since none of the three may import from
  // another.
  function defineHandlerProperty(target, type) {
    var current = null;
    Object.defineProperty(target, `on${type}`, {
      get: () => current,
      set: (fn) => {
        if (current) target.removeEventListener(type, current);
        current = typeof fn === 'function' ? fn : null;
        if (current) target.addEventListener(type, current);
      },
      configurable: true,
      enumerable: true,
    });
  }

  function createMockStreamSocket() {
    var socket = Object.assign(new EventTarget(), {
      readyState: CONNECTING,
      binaryType: 'arraybuffer',
      send: (value) => {
        onSend(String(value));
      },
      close: () => {
        socket.readyState = CLOSED;
        socket.dispatchEvent(new Event('close'));
      },
    });
    ['open', 'message', 'close', 'error'].forEach((type) => {
      defineHandlerProperty(socket, type);
    });
    return socket;
  }

  function fromBase64(base64) {
    var binary = atob(base64);
    var bytes = new Uint8Array(binary.length);
    var i;
    for (i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
  }

  // Decoded once per template frame (not per request) — `patchedCopy` below clones it fresh for
  // every request, since each one needs a different id burned into the same `idOffset`.
  function decodeTemplate(frame) {
    return { bytes: fromBase64(frame.base64), idOffset: frame.idOffset };
  }

  function patchedCopy(template, id) {
    var copy = template.bytes.slice();
    new DataView(copy.buffer).setInt32(template.idOffset, id, true);
    return copy.buffer;
  }

  // opId/tabId are renderer-generated per request and must not gate the match — mirrors
  // mockPort.ts's own matchKey exactly. `refresh` gets the same false/absent normalisation
  // mockRuntime.ts's own `canonical()` applies to the control channel (P58f-port finding): a live
  // stream.ts/state.ts call (e.g. runCount) never sets the key at all, but a fixture captured
  // straight from Go's own CountRequestWire always carries an explicit `refresh:false` — the wire
  // struct has no `omitempty` for it, unlike the TypeScript optional field it replaces.
  function matchKey(op, payload) {
    var rest;
    if (payload && typeof payload === 'object') {
      rest = Object.assign({}, payload);
      // JSON.stringify below drops undefined-valued keys same as it would a missing key, so
      // this keeps matchKey's own normalisation without the `delete` operator's deopt.
      rest.opId = undefined;
      rest.tabId = undefined;
      if (rest.refresh === false) rest.refresh = undefined;
      return `${op}:${JSON.stringify(rest)}`;
    }
    return `${op}:${JSON.stringify(payload)}`;
  }

  var pingTemplate = decodeTemplate(init.ping);
  var missTemplates = {};
  Object.keys(init.miss).forEach((op) => {
    missTemplates[op] = decodeTemplate(init.miss[op]);
  });

  var byKey = new Map();
  init.snapshots.forEach((snap) => {
    var key = matchKey(snap.op, snap.payload);
    var group = byKey.get(key) || [];
    group.push(Object.assign({}, snap, { template: decodeTemplate(snap.frame) }));
    byKey.set(key, group);
  });
  var cursors = new Map();
  var seen = [];
  var held = [];
  var socket;

  function onSend(raw) {
    var req = JSON.parse(raw);
    seen.push({
      op: req.op,
      payload: req.payload,
      opId: req.payload && typeof req.payload === 'object' ? req.payload.opId : undefined,
    });
    function respond(buf) {
      socket.dispatchEvent(new MessageEvent('message', { data: buf }));
    }
    // 'ping' (workbench/state/engine.ts's initEngineState) isn't a DATA_OP — it never appears in a
    // PortSnapshot fixture (types.ts's own doc comment: `op` is "a value from
    // shared/protocol/dataOps.ts's DATA_OP map") — so no spec's fixture array carries an entry
    // for it, and none needs to: every spec wants the engine pill to read 'ok', never asserts the
    // fake pid's actual value, so answering it unconditionally here removes one boilerplate line
    // from every single ported spec rather than adding a redundant fixture entry to each.
    if (req.op === 'ping') {
      respond(patchedCopy(pingTemplate, req.id));
      return;
    }
    var key = matchKey(req.op, req.payload);
    var group = byKey.get(key);
    var miss;
    if (!group) {
      miss = missTemplates[req.op];
      if (miss) respond(patchedCopy(miss, req.id));
      return;
    }
    var at = cursors.get(key) || 0;
    cursors.set(key, at + 1);
    var snap = group[Math.min(at, group.length - 1)];
    var replied = false;
    function reply() {
      if (replied) return;
      replied = true;
      respond(patchedCopy(snap.template, req.id));
    }
    // Studio-only (mockStream.ts's StudioPortSnapshot.untilCancel): the reply waits for
    // `__kiraReleaseCancelled`, which mockRuntime.ts fires when the renderer sends opsCancel. The
    // timeout is a safety net: a missing cancel then fails on the spec's log assertion, not a hang.
    if (snap.untilCancel) {
      held.push(reply);
      setTimeout(reply, 10000);
      return;
    }
    // Frontend-only (types.ts's PortSnapshot.delayMs) — see mockPort.ts's own comment.
    if (snap.delayMs) setTimeout(reply, snap.delayMs);
    else reply();
  }

  var g = globalThis;
  g._wails = g._wails || {};
  g._wails.streamFactory = () => {
    socket = createMockStreamSocket();
    // A real Stream() returns synchronously CONNECTING and opens asynchronously; queuing this on a
    // microtask (rather than opening synchronously before the caller's `onopen` assignment even
    // runs) matches that and exercises port.ts's own CONNECTING-gated send path (P57 D3), not a
    // shortcut around it.
    queueMicrotask(() => {
      socket.readyState = OPEN;
      socket.dispatchEvent(new Event('open'));
    });
    return socket;
  };
  g.__kiraStreamSeen = seen;
  g.__kiraPushFrame = (base64) => {
    socket.dispatchEvent(new MessageEvent('message', { data: fromBase64(base64).buffer }));
  };
  g.__kiraReleaseCancelled = () => {
    held.splice(0).forEach((reply) => {
      reply();
    });
  };
};
