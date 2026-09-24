import * as THREE from 'three';

import { HEIGHT_SCALE, PALETTE, TERRAIN } from './palette';

// Animated water: a translucent surface sheet over water tiles, one
// mesh per terrain chunk. The ripple is a vertex-shader sine wobble fed
// by a shared uTime uniform, so all chunks animate from one clock.

export interface WaterUniforms {
  time: { value: number };
}

export function waterUniforms(): WaterUniforms {
  return { time: { value: 0 } };
}

export function makeWaterMaterial(u: WaterUniforms): THREE.MeshPhongMaterial {
  const mat = new THREE.MeshPhongMaterial({
    color: 0xffffff,
    vertexColors: true,
    transparent: true,
    opacity: 0.78,
    shininess: 90,
    specular: new THREE.Color(0x8fb7d9),
    flatShading: true,
  });
  mat.onBeforeCompile = (shader: THREE.WebGLProgramParametersWithUniforms) => {
    shader.uniforms.uTime = u.time;
    // Two overlapping sine waves, sized to be clearly visible at RTS
    // zoom. waveD returns the analytic slope so normals (and the
    // specular glints on them) move with the swell.
    shader.vertexShader = shader.vertexShader
      .replace(
        '#include <common>',
        `#include <common>
uniform float uTime;
float waveH(vec2 p, float t) {
  return 0.13 * sin(t * 1.7 + p.x * 1.6 + p.y * 1.2)
       + 0.07 * sin(t * 0.83 + p.x * 0.6 - p.y * 0.8);
}
vec2 waveD(vec2 p, float t) {
  return vec2(
    0.13 * 1.6 * cos(t * 1.7 + p.x * 1.6 + p.y * 1.2)
      + 0.07 * 0.6 * cos(t * 0.83 + p.x * 0.6 - p.y * 0.8),
    0.13 * 1.2 * cos(t * 1.7 + p.x * 1.6 + p.y * 1.2)
      - 0.07 * 0.8 * cos(t * 0.83 + p.x * 0.6 - p.y * 0.8)
  );
}`,
      )
      .replace(
        '#include <begin_vertex>',
        `#include <begin_vertex>
transformed.y += waveH(transformed.xz, uTime);`,
      )
      .replace(
        '#include <beginnormal_vertex>',
        `#include <beginnormal_vertex>
{
  vec2 wsl = waveD(position.xz, uTime);
  objectNormal = normalize(objectNormal - vec3(wsl.x, 0.0, wsl.y));
}`,
      );
  };
  return mat;
}

export interface WaterWindow {
  x0: number;
  y0: number;
  w: number;
  h: number;
}

// waterLevelY derives one global sea-plane height from the map: the
// highest mean tile height among shallow tiles, plus a small margin so
// no shallow tile pokes through. Client-side only; it never feeds back
// into the simulation.
export function waterLevelY(size: number, tiles: Uint8Array, heights: Uint16Array): number {
  let maxShallow = -1;
  let maxDeep = -1;
  for (let ty = 0; ty < size; ty++) {
    for (let tx = 0; tx < size; tx++) {
      const tile = tiles[ty * size + tx];
      if (tile !== TERRAIN.Shallow && tile !== TERRAIN.Deep) continue;
      const sum =
        heights[ty * (size + 1) + tx] +
        heights[ty * (size + 1) + tx + 1] +
        heights[(ty + 1) * (size + 1) + tx] +
        heights[(ty + 1) * (size + 1) + tx + 1];
      const mean = sum / 4;
      if (tile === TERRAIN.Shallow && mean > maxShallow) maxShallow = mean;
      if (tile === TERRAIN.Deep && mean > maxDeep) maxDeep = mean;
    }
  }
  const top = maxShallow >= 0 ? maxShallow : maxDeep;
  if (top < 0) return 0;
  return (top + 0.4) * HEIGHT_SCALE;
}

// buildWaterWindow builds the surface geometry for the water tiles of
// one chunk window; null when the window holds no water. The surface is
// flat: every quad sits exactly on seaY. Callers attach it to a mesh
// with the shared water material.
export function buildWaterWindow(
  size: number,
  tiles: Uint8Array,
  win: WaterWindow,
  seaY: number,
): THREE.BufferGeometry | null {
  const { x0, y0, w, h } = win;
  const positions: number[] = [];
  const colors: number[] = [];
  const indices: number[] = [];
  const shallow = new THREE.Color(PALETTE.shallow);
  const deep = new THREE.Color(PALETTE.deep);
  const c = new THREE.Color();
  let v = 0;

  for (let ty = 0; ty < h; ty++) {
    for (let tx = 0; tx < w; tx++) {
      const mapX = x0 + tx;
      const mapY = y0 + ty;
      const tile = tiles[mapY * size + mapX];
      if (tile !== TERRAIN.Shallow && tile !== TERRAIN.Deep) continue;
      c.copy(tile === TERRAIN.Shallow ? shallow : deep);
      for (const [cx, cy] of [
        [0, 0],
        [1, 0],
        [1, 1],
        [0, 1],
      ]) {
        positions.push(mapX + cx, seaY, mapY + cy);
        colors.push(c.r, c.g, c.b);
      }
      indices.push(v, v + 2, v + 1, v, v + 3, v + 2);
      v += 4;
    }
  }
  if (v === 0) return null;

  const geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.BufferAttribute(new Float32Array(positions), 3));
  geo.setAttribute('color', new THREE.BufferAttribute(new Float32Array(colors), 3));
  geo.setIndex(new THREE.BufferAttribute(new Uint32Array(indices), 1));
  geo.computeVertexNormals();
  return geo;
}
