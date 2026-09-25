import { describe, expect, it } from 'vitest';

import { DAY_TICKS, DAWN_OFFSET, phaseOf, sunElevation } from './daylight';

describe('daylight cycle (mirrors sim/day)', () => {
  it('mirrors the Go constants', () => {
    expect(DAY_TICKS).toBe(9600);
    expect(DAWN_OFFSET).toBe(2880);
  });

  it('phase is in [0,1) and periodic', () => {
    for (const tick of [0, 1, 959, 4799, 9600, 9601, 1234567]) {
      const p = phaseOf(tick);
      expect(p).toBeGreaterThanOrEqual(0);
      expect(p).toBeLessThan(1);
      expect(phaseOf(tick + DAY_TICKS)).toBeCloseTo(p, 12);
    }
  });

  it('starts games in morning light', () => {
    expect(sunElevation(0)).toBeGreaterThan(0);
  });

  it('solar noon and midnight land where sim/day says', () => {
    expect(sunElevation(1920)).toBeCloseTo(1, 9);
    expect(sunElevation(6720)).toBeCloseTo(-1, 9);
  });

  it('handles pre-offset and negative ticks', () => {
    expect(phaseOf(-1)).toBeGreaterThanOrEqual(0);
    expect(phaseOf(-1)).toBeCloseTo(phaseOf(DAY_TICKS - 1), 12);
  });
});
