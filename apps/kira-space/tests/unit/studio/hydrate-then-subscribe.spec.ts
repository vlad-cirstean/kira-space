import { expect, test } from 'bun:test';
import { hydrateThenSubscribe } from '@workbench/state/hydrateThenSubscribe';

test('a value pushed during the snapshot await wins over the snapshot, later ones apply live', async () => {
  let push: (v: number) => void = () => {};
  const applied: number[] = [];
  await hydrateThenSubscribe({
    snapshot: async () => {
      push(2);
      return 1;
    },
    subscribe: (cb) => {
      push = cb;
      return () => {};
    },
    apply: (v) => applied.push(v),
  });
  push(3);
  expect(applied).toEqual([1, 2, 3]);
});
