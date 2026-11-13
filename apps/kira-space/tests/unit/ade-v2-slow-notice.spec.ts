import { afterEach, beforeEach, describe, expect, jest, test } from 'bun:test';
import { slowNotice } from '../../frontend/src/ade/v2/state/slowNotice';

// Timer against settle: a fast op shows nothing on success, a slow one resolves its loading notice,
// a double settle is ignored.
beforeEach(() => jest.useFakeTimers());
afterEach(() => jest.useRealTimers());

function setup() {
  const log: string[] = [];
  const notice = slowNotice({
    delayMs: 2000,
    show: () => log.push('show'),
    onSettle: (r) => log.push(`settle ok=${r.ok} shown=${r.shown} error=${r.error ?? ''}`),
  });
  return { log, notice };
}

describe('slowNotice', () => {
  test('settle before the delay never shows the loading notice', () => {
    const { log, notice } = setup();
    jest.advanceTimersByTime(1999);
    notice.settle(true);
    jest.advanceTimersByTime(5000);
    expect(log).toEqual(['settle ok=true shown=false error=']);
  });

  test('a failure before the delay still reports with shown false', () => {
    const { log, notice } = setup();
    jest.advanceTimersByTime(100);
    notice.settle(false, 'boom');
    expect(log).toEqual(['settle ok=false shown=false error=boom']);
  });

  test('settle after the delay reports the shown notice', () => {
    const { log, notice } = setup();
    jest.advanceTimersByTime(2000);
    notice.settle(true);
    expect(log).toEqual(['show', 'settle ok=true shown=true error=']);
  });

  test('a second settle is ignored', () => {
    const { log, notice } = setup();
    notice.settle(true);
    notice.settle(false, 'late');
    expect(log).toEqual(['settle ok=true shown=false error=']);
  });
});
