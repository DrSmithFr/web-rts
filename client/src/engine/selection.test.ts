import { describe, expect, it } from 'vitest';
import {
  MAX_SELECTION,
  Selection,
  normalizeRect,
  selectInRect,
  type ScreenPoint,
} from './selection';

function pt(id: number, sx: number, sy: number, onScreen = true): ScreenPoint {
  return { id, sx, sy, onScreen };
}

describe('Selection', () => {
  it('toggles membership', () => {
    const s = new Selection();
    s.toggle(3);
    s.toggle(4);
    expect(s.toArray()).toEqual([3, 4]);
    s.toggle(3);
    expect(s.toArray()).toEqual([4]);
  });

  it('caps size at MAX_SELECTION', () => {
    const s = new Selection();
    for (let i = 0; i < MAX_SELECTION + 10; i++) s.toggle(i);
    expect(s.size).toBe(MAX_SELECTION);
  });

  it('replace overwrites and caps', () => {
    const s = new Selection();
    s.replace([1, 2, 3]);
    expect(s.toArray()).toEqual([1, 2, 3]);
    s.replace(Array.from({ length: MAX_SELECTION + 5 }, (_, i) => i));
    expect(s.size).toBe(MAX_SELECTION);
  });
});

describe('selectInRect', () => {
  const rect = normalizeRect(10, 10, 30, 20);

  it('is inclusive on all edges', () => {
    const pts = [pt(1, 10, 10), pt(2, 30, 20), pt(3, 9, 15), pt(4, 15, 21)];
    expect(selectInRect(pts, rect)).toEqual([1, 2]);
  });

  it('skips off-screen points', () => {
    const pts = [pt(1, 15, 15, false), pt(2, 15, 15)];
    expect(selectInRect(pts, rect)).toEqual([2]);
  });

  it('normalizes inverted drags', () => {
    const r = normalizeRect(30, 20, 10, 10);
    expect(r).toEqual(rect);
    expect(selectInRect([pt(7, 20, 15)], r)).toEqual([7]);
  });
});
