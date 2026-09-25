import * as THREE from 'three';

// Builds ground meshes from sim heights: one quad per tile with whole-
// number corner heights, flat shading and per-tile colors. The map is
// split into fixed-size chunks so the renderer can frustum-cull and
// refresh fog per chunk; each chunk is one draw call.
// Visual blending is client-side only: a stable per-tile value jitter,
// rock color on steep slopes and sand where land touches water.
import { HEIGHT_SCALE, PALETTE, TERRAIN_COLORS, BIOME_COLORS } from './palette';
import { hash01 } from './hash';

export interface MeshWindow {
  x0?: number;
  y0?: number;
  w?: number;
  h?: number;
  stride?: number;
  // biomes is the optional per-tile biome byte layer (sim/biome ids);
  // when given, grass and forest tiles pick up their biome's tint so
  // biomes read at a glance.
  biomes?: Uint8Array;
}

// buildTerrainMesh builds (w x h) tiles starting at (x0, y0) of a map
// whose height array has stride+1 samples per row. Defaults: whole map.
export function buildTerrainMesh(
  size: number,
  tiles: Uint8Array,
  heights: Uint16Array,
  win?: MeshWindow,
): THREE.Mesh {
  const x0 = win?.x0 ?? 0;
  const y0 = win?.y0 ?? 0;
  const w = win?.w ?? size;
  const h = win?.h ?? size;
  const stride = win?.stride ?? size + 1;
  const biomes = win?.biomes;

  const vertexCount = w * h * 4;
  const positions = new Float32Array(vertexCount * 3);
  const colors = new Float32Array(vertexCount * 3);
  const indices = new Uint32Array(w * h * 6);

  const color = new THREE.Color();
  const rockC = new THREE.Color(PALETTE.rock);
  const sandC = new THREE.Color(PALETTE.sand);
  const scratch = new THREE.Color();
  const corner = [
    [0, 0],
    [1, 0],
    [1, 1],
    [0, 1],
  ];

  let v = 0;
  for (let ty = 0; ty < h; ty++) {
    for (let tx = 0; tx < w; tx++) {
      const mapX = x0 + tx;
      const mapY = y0 + ty;
      const tile = tiles[mapY * size + mapX];

      let hMin = Infinity;
      let hMax = -Infinity;
      for (const [cx, cy] of corner) {
        const hh = heights[(mapY + cy) * stride + (mapX + cx)];
        if (hh < hMin) hMin = hh;
        if (hh > hMax) hMax = hh;
      }

      color.setHex(TERRAIN_COLORS[tile] ?? 0xff00ff);
      // biome tint on open ground and forest, so regions read at a glance
      if (biomes && (tile === 0 || tile === 1)) {
        const bc = BIOME_COLORS[biomes[mapY * size + mapX]];
        if (bc !== undefined) {
          color.lerp(scratch.setHex(bc), 0.45);
        }
      }
      // steep ground reads as rock
      const slope = (hMax - hMin) * HEIGHT_SCALE;
      if (slope > 0.55) {
        const t = Math.min(1, (slope - 0.55) / 1.1) * 0.65;
        color.lerp(scratch.copy(rockC), t);
      }
      // land touching water picks up sand
      if (tile < 4 && tile !== 3) {
        const nb =
          (mapX > 0 && tiles[mapY * size + mapX - 1] >= 4 ? 1 : 0) +
          (mapX + 1 < size && tiles[mapY * size + mapX + 1] >= 4 ? 1 : 0) +
          (mapY > 0 && tiles[(mapY - 1) * size + mapX] >= 4 ? 1 : 0) +
          (mapY + 1 < size && tiles[(mapY + 1) * size + mapX] >= 4 ? 1 : 0);
        if (nb > 0) {
          color.lerp(scratch.copy(sandC), Math.min(0.5, 0.2 + 0.15 * nb));
        }
      }
      // stable per-tile brightness jitter to break flat color bands
      color.multiplyScalar(0.94 + 0.11 * hash01(mapX, mapY, 5));

      const base = v;
      for (const [cx, cy] of corner) {
        const hh = heights[(mapY + cy) * stride + (mapX + cx)];
        positions[v * 3] = mapX + cx;
        positions[v * 3 + 1] = hh * HEIGHT_SCALE;
        positions[v * 3 + 2] = mapY + cy;
        colors[v * 3] = color.r;
        colors[v * 3 + 1] = color.g;
        colors[v * 3 + 2] = color.b;
        v++;
      }
      const f = (base / 4) * 6;
      // Counter-clockwise seen from above so face normals point +y.
      indices[f] = base;
      indices[f + 1] = base + 2;
      indices[f + 2] = base + 1;
      indices[f + 3] = base;
      indices[f + 4] = base + 3;
      indices[f + 5] = base + 2;
    }
  }

  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3));
  geometry.setIndex(new THREE.BufferAttribute(indices, 1));
  geometry.computeVertexNormals();

  const material = new THREE.MeshLambertMaterial({
    vertexColors: true,
    flatShading: true,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.receiveShadow = true;
  return mesh;
}
