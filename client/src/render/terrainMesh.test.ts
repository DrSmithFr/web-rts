import * as THREE from 'three';
import { describe, expect, it } from 'vitest';
import { buildTerrainMesh } from './terrainMesh';

// A unit map: one grass tile, flat.
function flatMap(size: number): { tiles: Uint8Array; heights: Uint16Array } {
  return {
    tiles: new Uint8Array(size * size),
    heights: new Uint16Array((size + 1) * (size + 1)),
  };
}

describe('buildTerrainMesh', () => {
  it('produces upward-facing triangles', () => {
    const size = 2;
    const { tiles, heights } = flatMap(size);
    const mesh = buildTerrainMesh(size, tiles, heights);
    const geo = mesh.geometry;
    const pos = geo.getAttribute('position');
    const idx = geo.getIndex();
    const a = new THREE.Vector3();
    const b = new THREE.Vector3();
    const c = new THREE.Vector3();
    const ab = new THREE.Vector3();
    const ac = new THREE.Vector3();
    const n = new THREE.Vector3();
    for (let tri = 0; tri < idx!.count; tri += 3) {
      a.fromBufferAttribute(pos, idx!.getX(tri));
      b.fromBufferAttribute(pos, idx!.getX(tri + 1));
      c.fromBufferAttribute(pos, idx!.getX(tri + 2));
      ab.subVectors(b, a);
      ac.subVectors(c, a);
      n.crossVectors(ab, ac);
      expect(n.y).toBeGreaterThan(0);
    }
  });

  it('has one vertex and color per tile corner', () => {
    const size = 3;
    const { tiles, heights } = flatMap(size);
    const mesh = buildTerrainMesh(size, tiles, heights);
    expect(mesh.geometry.getAttribute('position').count).toBe(size * size * 4);
    expect(mesh.geometry.getAttribute('color').count).toBe(size * size * 4);
    expect(mesh.geometry.getIndex()!.count).toBe(size * size * 6);
  });

  it('encodes height into y', () => {
    const size = 1;
    const { tiles, heights } = flatMap(size);
    heights[0] = 10;
    const mesh = buildTerrainMesh(size, tiles, heights);
    const pos = mesh.geometry.getAttribute('position');
    expect(pos.getY(0)).toBeGreaterThan(3); // 10 * HEIGHT_SCALE
  });
});
