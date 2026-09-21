import { describe, expect, it } from 'vitest';

import { ControlGroups } from './groups';

const aliveAll = () => true;

describe('ControlGroups', () => {
  it('binds and recalls', () => {
    const g = new ControlGroups();
    g.bind(3, [10, 11]);
    expect(g.recall(3, aliveAll)).toEqual([10, 11]);
  });

  it('empty bind clears the slot', () => {
    const g = new ControlGroups();
    g.bind(3, [10]);
    g.bind(3, []);
    expect(g.recall(3, aliveAll)).toEqual([]);
  });

  it('add appends without duplicates', () => {
    const g = new ControlGroups();
    g.bind(1, [5, 6]);
    g.addTo(1, [6, 7]);
    expect(g.recall(1, aliveAll)).toEqual([5, 6, 7]);
  });

  it('drops dead members on recall', () => {
    const g = new ControlGroups();
    g.bind(2, [1, 2, 3]);
    const alive = (id: number) => id !== 2;
    expect(g.recall(2, alive)).toEqual([1, 3]);
    expect(g.recall(2, aliveAll)).toEqual([1, 3]);
  });

  it('forgets a fully dead group', () => {
    const g = new ControlGroups();
    g.bind(9, [1]);
    expect(g.recall(9, () => false)).toEqual([]);
    expect(g.recall(9, aliveAll)).toEqual([]);
  });
});
