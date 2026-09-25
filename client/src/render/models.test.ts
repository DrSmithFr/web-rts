import { describe, expect, it } from 'vitest';

import { buildingModel, unitModel, UNIT_DEFS } from './models';

const BUILDING_KEYS = ['hut', 'campfire', 'storehouse', 'barracks', 'market', 'forge', 'bank', 'wonder'];

function tris(geo: { getIndex(): { count: number } | null; getAttribute(name: string): { count: number } | null } | null): number {
  if (!geo) return 0;
  const idx = geo.getIndex();
  if (idx) return idx.count / 3;
  const pos = geo.getAttribute('position');
  return (pos?.count ?? 0) / 3;
}

describe('unitModel', () => {
  it.each(UNIT_DEFS)('unit %i builds within the 300-triangle budget', (def) => {
    const m = unitModel(def);
    expect(m.body).not.toBeNull();
    expect(m.anim.length).toBeGreaterThanOrEqual(2);
    let total = tris(m.body) + tris(m.insignia);
    for (const p of m.anim) total += tris(p.geo);
    expect(total).toBeLessThanOrEqual(300);
  });

  it('bakes vertex colors into every part', () => {
    for (const def of UNIT_DEFS) {
      const m = unitModel(def);
      for (const geo of [m.body, m.insignia, ...m.anim.map((p) => p.geo)]) {
        if (!geo) continue;
        expect(geo.getAttribute('color')).toBeTruthy();
      }
    }
  });

  it('caches models per def', () => {
    expect(unitModel(1)).toBe(unitModel(1));
  });
});

describe('buildingModel', () => {
  it.each(BUILDING_KEYS)('building "%s" builds within the 300-triangle budget', (key) => {
    const m = buildingModel(key);
    expect(m.structure).not.toBeNull();
    expect(tris(m.structure) + tris(m.insignia)).toBeLessThanOrEqual(300);
  });
});
