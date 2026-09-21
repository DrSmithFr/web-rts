import * as THREE from 'three';

import { FogVisible, groundTint, type FogView } from './fogView';
import { buildTerrainMesh } from './terrainMesh';

// GroundChunks splits the map into CHUNK-tile squares, one mesh each, so
// fog repaints and frustum culling stay local. Vertex colors bake the fog
// state per corner tile.

export const CHUNK = 32;

interface Chunk {
  mesh: THREE.Mesh;
  x0: number;
  y0: number;
  x1: number;
  y1: number;
  base: Float32Array;
  fogAt: Uint8Array;
}

export class GroundChunks {
  readonly group = new THREE.Group();
  readonly meshes: THREE.Mesh[] = [];
  private readonly chunks: Chunk[] = [];
  private fog: FogView | null = null;

  constructor(
    private readonly size: number,
    tiles: Uint8Array,
    heights: Uint16Array,
  ) {
    for (let y0 = 0; y0 < size; y0 += CHUNK) {
      for (let x0 = 0; x0 < size; x0 += CHUNK) {
        const w = Math.min(CHUNK, size - x0);
        const h = Math.min(CHUNK, size - y0);
        const mesh = buildTerrainMesh(size, tiles, heights, { x0, y0, w, h });
        const geo = mesh.geometry;
        const color = geo.getAttribute('color');
        const base = Float32Array.from(color.array as Float32Array);
        const chunk: Chunk = {
          mesh,
          x0,
          y0,
          x1: x0 + w,
          y1: y0 + h,
          base,
          fogAt: new Uint8Array((w + 1) * (h + 1)).fill(FogVisible),
        };
        this.chunks.push(chunk);
        this.meshes.push(mesh);
        this.group.add(mesh);
      }
    }
  }

  // setFog repaints only chunks whose corner fog states changed.
  setFog(fog: FogView): void {
    this.fog = fog;
    for (const c of this.chunks) {
      let dirty = false;
      const w = c.x1 - c.x0;
      const h = c.y1 - c.y0;
      for (let vy = 0; vy <= h; vy++) {
        for (let vx = 0; vx <= w; vx++) {
          const i = vy * (w + 1) + vx;
          const s = fog.at(c.x0 + vx, c.y0 + vy);
          if (s !== c.fogAt[i]) {
            c.fogAt[i] = s;
            dirty = true;
          }
        }
      }
      if (dirty) this.repaint(c);
    }
  }

  private repaint(c: Chunk): void {
    const w = c.x1 - c.x0;
    const color = c.mesh.geometry.getAttribute('color') as THREE.BufferAttribute;
    const arr = color.array as Float32Array;
    // Vertices are tile-quad corners (4 per tile, row-major within the
    // chunk window), not shared; sample the fog at each corner position.
    let v = 0;
    for (let ty = 0; ty < c.y1 - c.y0; ty++) {
      for (let tx = 0; tx < w; tx++) {
        for (const [cx, cy] of [
          [0, 0],
          [1, 0],
          [1, 1],
          [0, 1],
        ]) {
          const fi = (ty + cy) * (w + 1) + (tx + cx);
          const t = groundTint(c.fogAt[fi]);
          arr[v * 3] = c.base[v * 3] * t;
          arr[v * 3 + 1] = c.base[v * 3 + 1] * t;
          arr[v * 3 + 2] = c.base[v * 3 + 2] * t;
          v++;
        }
      }
    }
    color.needsUpdate = true;
  }

  // cull hides chunks outside the camera frustum.
  cull(camera: THREE.Camera): void {
    const frustum = new THREE.Frustum();
    const proj = new THREE.Matrix4().multiplyMatrices(
      camera.projectionMatrix,
      camera.matrixWorldInverse,
    );
    frustum.setFromProjectionMatrix(proj);
    for (const c of this.chunks) {
      c.mesh.visible = frustum.intersectsObject(c.mesh);
    }
  }
}
