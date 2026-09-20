import { describe, expect, it } from 'vitest';
import { MAX_CATCHUP_TICKS, SIM_TICK_MS, TimeAccumulator } from './loop';

describe('TimeAccumulator', () => {
  const make = () => new TimeAccumulator(SIM_TICK_MS, MAX_CATCHUP_TICKS);

  it('runs 20 Hz on steady frames', () => {
    const acc = make();
    let total = 0;
    for (let i = 0; i < 60; i++) {
      total += acc.advance(50);
    }
    expect(total).toBe(60);
  });

  it('returns zero on frames shorter than a tick', () => {
    const acc = make();
    expect(acc.advance(20)).toBe(0);
    expect(acc.advance(20)).toBe(0);
    expect(acc.advance(20)).toBe(1);
  });

  it('keeps the fractional remainder across frames', () => {
    const acc = make();
    expect(acc.advance(120)).toBe(2);
    expect(acc.advance(40)).toBe(1);
    expect(acc.advance(10)).toBe(0);
    expect(acc.advance(40)).toBe(1);
  });

  it('caps catch-up and drops stale time', () => {
    const acc = make();
    expect(acc.advance(30_000)).toBe(MAX_CATCHUP_TICKS);
    expect(acc.advance(0)).toBe(0);
    expect(acc.advance(SIM_TICK_MS)).toBe(1);
  });

  it('ignores negative elapsed', () => {
    const acc = make();
    expect(acc.advance(-500)).toBe(0);
    expect(acc.advance(SIM_TICK_MS)).toBe(1);
  });
});
