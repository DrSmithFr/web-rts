import { describe, expect, it } from 'vitest';

import { hash01, hash2 } from './hash';

describe('hash2', () => {
  it('is stable across calls', () => {
    expect(hash2(123, 456, 7)).toBe(hash2(123, 456, 7));
    expect(hash2(123, 456, 7)).not.toBe(hash2(123, 456, 8));
    expect(hash2(123, 456, 7)).not.toBe(hash2(124, 456, 7));
  });

  it('stays a 32-bit unsigned integer', () => {
    for (let x = -5; x < 1005; x += 13) {
      for (let y = -5; y < 1005; y += 17) {
        const h = hash2(x, y, 3);
        expect(Number.isInteger(h)).toBe(true);
        expect(h).toBeGreaterThanOrEqual(0);
        expect(h).toBeLessThanOrEqual(0xffffffff);
      }
    }
  });

  it('spreads values over [0,1)', () => {
    let lo = 1;
    let hi = 0;
    for (let i = 0; i < 4000; i++) {
      const v = hash01(i, i * 7 + 11, 5);
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThan(1);
      if (v < lo) lo = v;
      if (v > hi) hi = v;
    }
    expect(lo).toBeLessThan(0.01);
    expect(hi).toBeGreaterThan(0.99);
  });
});
