import * as THREE from 'three';

import { FogVisible, groundTint, type FogView } from './fogView';
import { buildTerrainMesh } from './terrainMesh';
import { buildWaterWindow, makeWaterMaterial, waterLevelY, waterUniforms } from './water';
import { buildScatterWindow } from './scatter';

// GroundChunks splits the map into CHUNK-tile squares, one mesh each, so
// fog repaints and frustum culling stay local. Vertex colors bake the fog
// state per corner tile. Each chunk also carries a translucent water
// surface (shared animated material) and merged decor scatter.

export const CHUNK = 32;

interface Chunk {
  mesh: THREE.Mesh;
  water: THREE.Mesh | null;
  decor: THREE.Mesh | null;
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
  private readonly waterMat: THREE.MeshPhongMaterial;
  private readonly decorMat: THREE.MeshLambertMaterial;
  private readonly uniforms = waterUniforms();

  constructor(
    size: number,
    tiles: Uint8Array,
    heights: Uint16Array,
    blocked?: Set<number>,
    biomes?: Uint8Array,
  ) {
    this.waterMat = makeWaterMaterial(this.uniforms);
    this.decorMat = new THREE.MeshLambertMaterial({ vertexColors: true, flatShading: true });
    const seaY = waterLevelY(size, tiles, heights);

    for (let y0 = 0; y0 < size; y0 += CHUNK) {
      for (let x0 = 0; x0 < size; x0 += CHUNK) {
        const w = Math.min(CHUNK, size - x0);
        const h = Math.min(CHUNK, size - y0);
        const mesh = buildTerrainMesh(size, tiles, heights, { x0, y0, w, h, biomes });
        const geo = mesh.geometry;
        const color = geo.getAttribute('color');
        const base = Float32Array.from(color.array as Float32Array);

        let water: THREE.Mesh | null = null;
        const waterGeo = buildWaterWindow(size, tiles, { x0, y0, w, h }, seaY);
        if (waterGeo) {
          water = new THREE.Mesh(waterGeo, this.waterMat);
          water.renderOrder = 1;
        }
        let decor: THREE.Mesh | null = null;
        const decorGeo = buildScatterWindow(size, tiles, heights, { x0, y0, w, h }, blocked);
        if (decorGeo) {
          decor = new THREE.Mesh(decorGeo, this.decorMat);
        }

        const chunk: Chunk = {
          mesh,
          water,
          decor,
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
        if (water) this.group.add(water);
        if (decor) this.group.add(decor);
      }
    }
  }

  // setTime drives the shared water ripple clock.
  setTime(seconds: number): void {
    this.uniforms.time.value = seconds;
  }

  // setFog repaints only chunks whose corner fog states changed.
  setFog(fog: FogView): void {
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
      const hit = frustum.intersectsObject(c.mesh);
      c.mesh.visible = hit;
      if (c.water) c.water.visible = hit;
      if (c.decor) c.decor.visible = frustum.intersectsObject(c.decor);
    }
  }
}
