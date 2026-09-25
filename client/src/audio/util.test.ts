import { describe, expect, it } from 'vitest';

import { Throttle, clampPct } from './util';

describe('clampPct', () => {
  it('clamps and rounds to the 0..100 range', () => {
    expect(clampPct(-5)).toBe(0);
    expect(clampPct(105)).toBe(100);
    expect(clampPct(66.6)).toBe(67);
    expect(clampPct(NaN)).toBe(0);
  });
});

describe('Throttle', () => {
  it('allows one event per window', () => {
    const t = new Throttle(100);
    expect(t.allow(0)).toBe(true);
    expect(t.allow(50)).toBe(false);
    expect(t.allow(99)).toBe(false);
    expect(t.allow(100)).toBe(true);
    expect(t.allow(180)).toBe(false);
    expect(t.allow(200)).toBe(true);
  });
});
