import type { Locator } from '@playwright/test';
import { expect } from '@playwright/test';

// Element geometry for the approval alignment specs: every check is within 1px.
export const TOL = 1;

export async function box(l: Locator) {
  const b = await l.boundingBox();
  if (!b) throw new Error(`no box for ${l}`);
  return b;
}
export const left = async (l: Locator) => (await box(l)).x;
export const right = async (l: Locator) => {
  const b = await box(l);
  return b.x + b.width;
};
export function near(actual: number, expected: number, what: string) {
  expect(Math.abs(actual - expected), `${what}: ${actual} vs ${expected}`).toBeLessThanOrEqual(TOL);
}
