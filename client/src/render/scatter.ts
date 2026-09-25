import * as THREE from 'three';

import { Builder } from './models';
import { HEIGHT_SCALE, PALETTE, TERRAIN } from './palette';
import { hash01 } from './hash';

// Decorative scatter merged per terrain chunk: canopy trees on forest
// tiles, tufts and flowers on grass, pebbles on rock, reeds along the
// shore. Placement comes from a stable integer hash of tile coordinates,
// so decor never changes and never reveals hidden information.

const TREE_CHANCE = 0.16;
const TUFT_CHANCE = 0.013;
const FLOWER_CHANCE = 0.004;
const PEBBLE_CHANCE = 0.02;
const REED_CHANCE = 0.09;

export interface ScatterWindow {
  x0: number;
  y0: number;
  w: number;
  h: number;
}

// buildScatterWindow returns merged vertex-colored decor geometry for a
// chunk window (null when empty). blocked holds tile indices occupied by
// harvestable nodes so decor never grows through them.
export function buildScatterWindow(
  size: number,
  tiles: Uint8Array,
  heights: Uint16Array,
  win: ScatterWindow,
  blocked?: Set<number>,
): THREE.BufferGeometry | null {
  const { x0, y0, w, h } = win;
  const b = new Builder();
  const yAt = (tx: number, ty: number): number => heights[ty * (size + 1) + tx] * HEIGHT_SCALE;

  for (let ty = 0; ty < h; ty++) {
    for (let tx = 0; tx < w; tx++) {
      const mapX = x0 + tx;
      const mapY = y0 + ty;
      const tile = tiles[mapY * size + mapX];
      if (blocked && blocked.has(mapY * size + mapX)) continue;
      const cx = mapX + 0.25 + 0.5 * hash01(mapX, mapY, 11);
      const cz = mapY + 0.25 + 0.5 * hash01(mapX, mapY, 12);
      const y = yAt(mapX, mapY);

      if (tile === TERRAIN.Forest && hash01(mapX, mapY, 21) < TREE_CHANCE) {
        const s = 0.85 + 0.5 * hash01(mapX, mapY, 22);
        b.cyl(0.06 * s, 0.09 * s, 0.45 * s, cx, y + 0.22 * s, cz, PALETTE.trunk, 5);
        b.cone(0.42 * s, 1.0 * s, cx, y + (0.45 + 0.5) * s, cz, PALETTE.leaf, 6, 0, hash01(mapX, mapY, 23) * 6.28);
        continue;
      }
      if (tile === TERRAIN.Grass) {
        const r = hash01(mapX, mapY, 31);
        if (r < TUFT_CHANCE) {
          b.cone(0.05, 0.16, cx, y + 0.08, cz, PALETTE.tuft, 4, 0, hash01(mapX, mapY, 32) * 6.28);
          b.cone(0.04, 0.12, cx + 0.07, y + 0.06, cz - 0.05, PALETTE.tuft, 4);
        } else if (r < TUFT_CHANCE + FLOWER_CHANCE) {
          b.cyl(0.01, 0.01, 0.12, cx, y + 0.06, cz, PALETTE.reed, 4);
          b.box(0.05, 0.03, 0.05, cx, y + 0.13, cz, PALETTE.flower);
        }
        continue;
      }
      if (tile === TERRAIN.Rock && hash01(mapX, mapY, 41) < PEBBLE_CHANCE) {
        b.box(0.12, 0.08, 0.1, cx, y + 0.04, cz, PALETTE.stone, 0, hash01(mapX, mapY, 42) * 6.28);
        continue;
      }
      if (tile === TERRAIN.Sand || tile === TERRAIN.Grass || tile === TERRAIN.Forest) {
        const shore =
          (mapX > 0 && tiles[mapY * size + mapX - 1] >= 4 ? 1 : 0) +
          (mapX + 1 < size && tiles[mapY * size + mapX + 1] >= 4 ? 1 : 0) +
          (mapY > 0 && tiles[(mapY - 1) * size + mapX] >= 4 ? 1 : 0) +
          (mapY + 1 < size && tiles[(mapY + 1) * size + mapX] >= 4 ? 1 : 0);
        if (shore > 0 && hash01(mapX, mapY, 51) < REED_CHANCE) {
          b.cone(0.03, 0.3, cx, y + 0.15, cz, PALETTE.reed, 4, 0.1);
        }
      }
    }
  }
  return b.merge();
}
