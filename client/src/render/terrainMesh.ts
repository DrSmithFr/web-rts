import * as THREE from 'three';

// Builds ground meshes from sim heights: one quad per tile with whole-
// number corner heights, flat shading and per-tile colors. The map is
// split into fixed-size chunks so the renderer can frustum-cull and
// refresh fog per chunk; each chunk is one draw call.
import { HEIGHT_SCALE, TERRAIN_COLORS } from './palette';

export interface MeshWindow {
  x0?: number;
  y0?: number;
  w?: number;
  h?: number;
  stride?: number;
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

  const vertexCount = w * h * 4;
  const positions = new Float32Array(vertexCount * 3);
  const colors = new Float32Array(vertexCount * 3);
  const indices = new Uint32Array(w * h * 6);

  const color = new THREE.Color();
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
      color.setHex(TERRAIN_COLORS[tile] ?? 0xff00ff);
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
