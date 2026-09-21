import { describe, expect, it } from 'vitest';

import { HEIGHT_SCALE } from './palette';
import { heightSampler } from './height';

function rampGrid(size: number): Uint16Array {
  const heights = new Uint16Array((size + 1) * (size + 1));
  for (let z = 0; z <= size; z++) {
    for (let x = 0; x <= size; x++) {
      heights[z * (size + 1) + x] = x * 100 + z * 200;
    }
  }
  return heights;
}

describe('heightSampler', () => {
  const size = 4;
  const sample = heightSampler(size, rampGrid(size));

  it('returns exact corner heights', () => {
    expect(sample(2, 3)).toBe((2 * 100 + 3 * 200) * HEIGHT_SCALE);
  });

  it('interpolates linearly between corners', () => {
    const h00 = sample(1, 1);
    const h10 = sample(2, 1);
    const mid = sample(1.5, 1);
    expect(mid).toBeCloseTo((h00 + h10) / 2, 6);
    expect(sample(1.25, 1)).toBeCloseTo(h00 + (h10 - h00) * 0.25, 6);
  });

  it('is continuous across tile boundaries', () => {
    const before = sample(2 - 1e-9, 2 - 1e-9);
    const at = sample(2, 2);
    const after = sample(2 + 1e-9, 2 + 1e-9);
    expect(Math.abs(before - at)).toBeLessThan(1e-6);
    expect(Math.abs(after - at)).toBeLessThan(1e-6);
  });

  it('clamps out-of-bounds coordinates', () => {
    expect(sample(-3, -3)).toBe(0);
    expect(sample(size + 5, size + 5)).toBe((size * 100 + size * 200) * HEIGHT_SCALE);
  });
});
