import { describe, expect, it } from 'vitest';

import { Interpolated } from './interpolate';
import { TimeAccumulator } from './loop';

describe('Interpolated', () => {
  it('holds first-seen entities at their current position', () => {
    const it0 = new Interpolated();
    it0.commit([[1, { x: 5, y: 7 }]]);
    expect(it0.get(1, 0)).toEqual({ x: 5, y: 7 });
    expect(it0.get(2, 0.5)).toBeUndefined();
  });

  it('blends linearly between snapshots', () => {
    const it0 = new Interpolated();
    it0.commit([[1, { x: 0, y: 0 }]]);
    it0.commit([[1, { x: 10, y: 4 }]]);
    expect(it0.get(1, 0)).toEqual({ x: 0, y: 0 });
    expect(it0.get(1, 0.5)).toEqual({ x: 5, y: 2 });
    expect(it0.get(1, 1)).toEqual({ x: 10, y: 4 });
  });

  it('clamps alpha to [0, 1]', () => {
    const it0 = new Interpolated();
    it0.commit([[1, { x: 0, y: 0 }]]);
    it0.commit([[1, { x: 2, y: 0 }]]);
    expect(it0.get(1, -3)).toEqual({ x: 0, y: 0 });
    expect(it0.get(1, 9)).toEqual({ x: 2, y: 0 });
  });

  it('drops entities that vanish', () => {
    const it0 = new Interpolated();
    it0.commit([[1, { x: 0, y: 0 }]]);
    it0.commit([[2, { x: 1, y: 1 }]]);
    expect(it0.get(1, 1)).toBeUndefined();
    expect(it0.get(2, 1)).toEqual({ x: 1, y: 1 });
  });
});

describe('accumulator fraction', () => {
  it('reports the partial tick for interpolation', () => {
    const acc = new TimeAccumulator(50, 5);
    expect(acc.fraction()).toBeCloseTo(0);
    acc.advance(75); // one tick + 25ms remainder
    expect(acc.fraction()).toBeCloseTo(0.5);
    acc.advance(25);
    expect(acc.fraction()).toBeCloseTo(0);
  });
});
