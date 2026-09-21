import * as THREE from 'three';

// Builds the ground mesh from sim heights: one quad per tile with whole-
// number corner heights, flat shading and per-tile colors. A 200x200 map
// stays a single draw call.
import { HEIGHT_SCALE, TERRAIN_COLORS } from './palette';

export function buildTerrainMesh(
  size: number,
  tiles: Uint8Array,
  heights: Uint16Array,
): THREE.Mesh {
  const vertexCount = size * size * 4;
  const positions = new Float32Array(vertexCount * 3);
  const colors = new Float32Array(vertexCount * 3);
  const indices = new Uint32Array(size * size * 6);

  const color = new THREE.Color();
  const corner = [
    [0, 0],
    [1, 0],
    [1, 1],
    [0, 1],
  ];

  let v = 0;
  for (let ty = 0; ty < size; ty++) {
    for (let tx = 0; tx < size; tx++) {
      const tile = tiles[ty * size + tx];
      color.setHex(TERRAIN_COLORS[tile] ?? 0xff00ff);
      const base = v;
      for (const [cx, cy] of corner) {
        const h = heights[(ty + cy) * (size + 1) + (tx + cx)];
        positions[v * 3] = tx + cx;
        positions[v * 3 + 1] = h * HEIGHT_SCALE;
        positions[v * 3 + 2] = ty + cy;
        colors[v * 3] = color.r;
        colors[v * 3 + 1] = color.g;
        colors[v * 3 + 2] = color.b;
        v++;
      }
      const f = (base / 4) * 6;
      indices[f] = base;
      indices[f + 1] = base + 1;
      indices[f + 2] = base + 2;
      indices[f + 3] = base;
      indices[f + 4] = base + 2;
      indices[f + 5] = base + 3;
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
