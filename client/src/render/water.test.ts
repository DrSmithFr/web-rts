import * as THREE from 'three';
import { describe, expect, it } from 'vitest';

import { HEIGHT_SCALE, TERRAIN } from './palette';
import { buildWaterWindow, makeWaterMaterial, waterLevelY, waterUniforms } from './water';

const S = 3;

function flatMap(fill = 10): { tiles: Uint8Array; heights: Uint16Array } {
  return {
    tiles: new Uint8Array(S * S),
    heights: new Uint16Array((S + 1) * (S + 1)).fill(fill),
  };
}

describe('waterLevelY', () => {
  it('is one flat level: max shallow mean plus margin', () => {
    const { tiles, heights } = flatMap();
    tiles[4] = TERRAIN.Shallow; // mean corner 10
    tiles[5] = TERRAIN.Shallow; // mean corner 10
    expect(waterLevelY(S, tiles, heights)).toBeCloseTo((10 + 0.4) * HEIGHT_SCALE);
  });

  it('follows the highest shallow tile, not its own chunk', () => {
    const { tiles, heights } = flatMap(10);
    heights[0] = 30;
    heights[1] = 30;
    heights[S + 1] = 30;
    heights[S + 2] = 30;
    tiles[0] = TERRAIN.Shallow; // corners all 30
    tiles[8] = TERRAIN.Shallow; // corners all 10
    expect(waterLevelY(S, tiles, heights)).toBeCloseTo((30 + 0.4) * HEIGHT_SCALE);
  });

  it('falls back to deep tiles when there is no shoreline', () => {
    const { tiles, heights } = flatMap(4);
    tiles[4] = TERRAIN.Deep;
    expect(waterLevelY(S, tiles, heights)).toBeCloseTo((4 + 0.4) * HEIGHT_SCALE);
  });

  it('returns zero for a waterless map', () => {
    const { tiles, heights } = flatMap();
    expect(waterLevelY(S, tiles, heights)).toBe(0);
  });
});

describe('buildWaterWindow', () => {
  it('puts every water quad at exactly sea level', () => {
    const { tiles } = flatMap(10);
    tiles[0] = TERRAIN.Shallow;
    tiles[8] = TERRAIN.Deep;
    const seaY = 25 * HEIGHT_SCALE;
    const geo = buildWaterWindow(S, tiles, { x0: 0, y0: 0, w: S, h: S }, seaY);
    expect(geo).not.toBeNull();
    const pos = geo!.getAttribute('position');
    expect(pos.count).toBe(8); // two quads
    for (let i = 0; i < pos.count; i++) expect(pos.getY(i)).toBeCloseTo(seaY);
  });

  it('returns null for a waterless window', () => {
    const { tiles } = flatMap();
    expect(buildWaterWindow(S, tiles, { x0: 0, y0: 0, w: S, h: S }, 1)).toBeNull();
  });
});

describe('makeWaterMaterial', () => {
  it('patches the phong vertex shader: uTime, wave displacement, slope normals', () => {
    const u = waterUniforms();
    const mat = makeWaterMaterial(u);
    const shader = {
      uniforms: {} as Record<string, unknown>,
      vertexShader: THREE.ShaderLib.phong.vertexShader,
    } as unknown as THREE.WebGLProgramParametersWithUniforms;
    mat.onBeforeCompile(shader, null as never);

    expect(shader.uniforms.uTime).toBe(u.time);
    const src = shader.vertexShader as string;
    // includes stay in place (three resolves them after onBeforeCompile);
    // the wave code must land right after them
    expect(src).toContain('transformed.y += waveH(transformed.xz, uTime);');
    expect(src).toContain('objectNormal = normalize(objectNormal - vec3(wsl.x, 0.0, wsl.y));');
    expect(src).toContain('uniform float uTime;');
    expect(src.indexOf('transformed.y += waveH')).toBeGreaterThan(src.indexOf('#include <begin_vertex>'));
    expect(src.indexOf('vec2 wsl = waveD')).toBeGreaterThan(src.indexOf('#include <beginnormal_vertex>'));
    // helpers defined exactly once, before main()
    expect(src.split('float waveH(').length - 1).toBe(1);
    expect(src.split('vec2 waveD(').length - 1).toBe(1);
    expect(src.indexOf('float waveH(')).toBeLessThan(src.indexOf('void main()'));
  });
});
