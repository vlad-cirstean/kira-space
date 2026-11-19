// Deep-freezes every intrinsic reachable from the globals named in `names`, plus `kira`, then the
// global object. Cycle-safe. Run once per VM, after bindings are installed and before the script.
(function __kira_freeze(g, names, kira) {
  const seen = new Set();
  const queue = [];
  const add = (v) => {
    if (((typeof v === 'object' && v !== null) || typeof v === 'function') && !seen.has(v)) {
      seen.add(v);
      queue.push(v);
    }
  };
  for (const n of names) {
    add(g[n]);
  }
  add(kira);
  while (queue.length > 0) {
    const o = queue.pop();
    for (const k of Reflect.ownKeys(o)) {
      const d = Reflect.getOwnPropertyDescriptor(o, k);
      if (d) {
        add(d.value);
        add(d.get);
        add(d.set);
      }
    }
    add(Reflect.getPrototypeOf(o));
    Object.freeze(o);
  }
  Object.freeze(g);
});
