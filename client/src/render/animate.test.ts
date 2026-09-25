import { describe, expect, it } from 'vitest';

import { angDiff, stepYaw, walkAmp, walkPhase } from './animate';

describe('angDiff', () => {
  it('returns the shortest signed turn', () => {
    expect(angDiff(0, Math.PI / 2)).toBeCloseTo(Math.PI / 2);
    expect(angDiff(Math.PI / 2, 0)).toBeCloseTo(-Math.PI / 2);
    // 3PI/2 clockwise is the short way to +PI/2 and vice versa
    expect(angDiff(0, Math.PI * 1.5)).toBeCloseTo(-Math.PI / 2);
    expect(angDiff(0, -Math.PI * 1.5)).toBeCloseTo(Math.PI / 2);
  });
});

describe('stepYaw', () => {
  it('rate-limits rotation', () => {
    const y = stepYaw(0, 1.0, 0.05, 10);
    expect(y).toBeCloseTo(0.5);
  });

  it('snaps to target when within budget', () => {
    expect(stepYaw(0, 0.1, 0.1, 10)).toBeCloseTo(0.1);
  });

  it('wraps around the long way correctly', () => {
    let y = 0;
    for (let i = 0; i < 60; i++) y = stepYaw(y, Math.PI * 1.9, 0.05, 10);
    // from 0 the shortest path to 1.9PI goes negative
    expect(y).toBeLessThan(0.2);
    expect(Math.abs(angDiff(y, Math.PI * 1.9))).toBeLessThan(0.05);
  });

  it('keeps yaw when the target is NaN', () => {
    expect(stepYaw(1.23, NaN, 0.05)).toBeCloseTo(1.23);
  });
});

describe('walkPhase', () => {
  it('advances with distance travelled', () => {
    expect(walkPhase(0, 0)).toBe(0);
    expect(walkPhase(1, 0)).toBe(1);
    expect(walkPhase(0, 1)).toBeCloseTo(Math.PI * 2);
  });
});

describe('walkAmp', () => {
  it('eases up while moving and back to zero at rest', () => {
    let a = 0;
    for (let i = 0; i < 30; i++) a = walkAmp(a, 1, 0.05);
    expect(a).toBeGreaterThan(0.3);
    expect(a).toBeLessThanOrEqual(0.55);
    for (let i = 0; i < 60; i++) a = walkAmp(a, 0, 0.05);
    expect(a).toBeLessThan(0.01);
  });

  it('never exceeds the amplitude cap', () => {
    let a = 0;
    for (let i = 0; i < 200; i++) a = walkAmp(a, 50, 0.05);
    expect(a).toBeLessThanOrEqual(0.55);
  });
});
