import * as THREE from 'three';

// Transient effects: spark bursts (hits, deaths, harvest pops) and RTS
// order markers (expanding ground rings). Instanced pools, additive
// blending, no allocation in the update loop.

const PARTICLE_CAP = 320;
const RING_CAP = 16;
const GRAVITY = 7.5;

interface Particle {
  x: number;
  y: number;
  z: number;
  vx: number;
  vy: number;
  vz: number;
  ttl: number;
  max: number;
  r: number;
  g: number;
  b: number;
  size: number;
}

interface Ring {
  x: number;
  z: number;
  y: number;
  ttl: number;
  max: number;
  r: number;
  g: number;
  b: number;
}

export interface FX {
  group: THREE.Group;
  // burst spawns n sparks at a world position in an additive color.
  burst(x: number, y: number, z: number, color: number, n: number, spread?: number): void;
  // ring drops an expanding marker ring on the ground.
  ring(x: number, z: number, color: number): void;
  update(dt: number): void;
}

export function buildFX(ground?: (x: number, z: number) => number): FX {
  const group = new THREE.Group();

  const sparkGeo = new THREE.BoxGeometry(1, 1, 1);
  const sparkMat = new THREE.MeshBasicMaterial({
    color: 0xffffff,
    transparent: true,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
  });
  const sparks = new THREE.InstancedMesh(sparkGeo, sparkMat, PARTICLE_CAP);
  sparks.frustumCulled = false;
  sparks.count = PARTICLE_CAP;
  group.add(sparks);

  const ringGeo = new THREE.TorusGeometry(1, 0.09, 4, 32);
  ringGeo.rotateX(-Math.PI / 2);
  const ringMat = new THREE.MeshBasicMaterial({
    color: 0xffffff,
    transparent: true,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
  });
  const ringMesh = new THREE.InstancedMesh(ringGeo, ringMat, RING_CAP);
  ringMesh.frustumCulled = false;
  ringMesh.count = RING_CAP;
  group.add(ringMesh);

  const parts: Particle[] = [];
  const rings: Ring[] = [];
  const matrix = new THREE.Matrix4();
  const color = new THREE.Color();
  const zero = new THREE.Vector3(0.0001, 0.0001, 0.0001);

  const hideAll = (): void => {
    for (let i = 0; i < PARTICLE_CAP; i++) {
      matrix.makeTranslation(0, -50, 0);
      matrix.scale(zero);
      sparks.setMatrixAt(i, matrix);
    }
    sparks.instanceMatrix.needsUpdate = true;
  };
  hideAll();
  const hideRings = (): void => {
    for (let i = 0; i < RING_CAP; i++) {
      matrix.compose(new THREE.Vector3(0, -50, 0), new THREE.Quaternion(), zero);
      ringMesh.setMatrixAt(i, matrix);
    }
    ringMesh.instanceMatrix.needsUpdate = true;
  };
  hideRings();
  sparks.setColorAt(0, color.setHex(0xffffff));

  return {
    group,
    burst(x, y, z, hex, n, spread = 1): void {
      color.setHex(hex);
      for (let i = 0; i < n; i++) {
        if (parts.length >= PARTICLE_CAP) break;
        const a = Math.random() * Math.PI * 2;
        const sp = (0.6 + Math.random() * 1.6) * spread;
        parts.push({
          x,
          y: y + 0.1,
          z,
          vx: Math.cos(a) * sp,
          vy: 1.6 + Math.random() * 2.2,
          vz: Math.sin(a) * sp,
          ttl: 0.35 + Math.random() * 0.25,
          max: 0.6,
          r: color.r,
          g: color.g,
          b: color.b,
          size: 0.05 + Math.random() * 0.05,
        });
      }
    },
    ring(x, z, hex): void {
      if (rings.length >= RING_CAP) rings.shift();
      color.setHex(hex);
      // ground markers sit on the terrain surface, not on y=0
      const y = (ground ? ground(x, z) : 0) + 0.06;
      rings.push({ x, z, y, ttl: 0.55, max: 0.55, r: color.r, g: color.g, b: color.b });
    },
    update(dt): void {
      // particles
      let n = 0;
      for (let i = 0; i < parts.length; ) {
        const p = parts[i];
        p.ttl -= dt;
        if (p.ttl <= 0) {
          parts[i] = parts[parts.length - 1];
          parts.pop();
          continue;
        }
        p.vy -= GRAVITY * dt;
        p.x += p.vx * dt;
        p.y += p.vy * dt;
        p.z += p.vz * dt;
        if (p.y < 0.01) {
          p.y = 0.01;
          p.vy = Math.abs(p.vy) * 0.3;
        }
        const f = Math.max(0, p.ttl / p.max);
        const s = p.size * (0.5 + f);
        matrix.compose(new THREE.Vector3(p.x, p.y, p.z), new THREE.Quaternion(), new THREE.Vector3(s, s, s));
        sparks.setMatrixAt(n, matrix);
        color.setRGB(p.r * f, p.g * f, p.b * f);
        sparks.setColorAt(n, color);
        n++;
        i++;
      }
      for (let i = n; i < PARTICLE_CAP; i++) {
        matrix.compose(new THREE.Vector3(0, -50, 0), new THREE.Quaternion(), zero);
        sparks.setMatrixAt(i, matrix);
        color.setRGB(0, 0, 0);
        sparks.setColorAt(i, color);
      }
      sparks.instanceMatrix.needsUpdate = true;
      if (sparks.instanceColor) sparks.instanceColor.needsUpdate = true;

      // rings
      let m = 0;
      for (let i = 0; i < rings.length; ) {
        const r = rings[i];
        r.ttl -= dt;
        if (r.ttl <= 0) {
          rings[i] = rings[rings.length - 1];
          rings.pop();
          continue;
        }
        const f = r.ttl / r.max;
        const s = 0.35 + (1 - f) * 0.85;
        matrix.compose(new THREE.Vector3(r.x, r.y, r.z), new THREE.Quaternion(), new THREE.Vector3(s, 1, s));
        ringMesh.setMatrixAt(m, matrix);
        color.setRGB(r.r * f, r.g * f, r.b * f);
        ringMesh.setColorAt(m, color);
        m++;
        i++;
      }
      for (let i = m; i < RING_CAP; i++) {
        matrix.compose(new THREE.Vector3(0, -50, 0), new THREE.Quaternion(), zero);
        ringMesh.setMatrixAt(i, matrix);
      }
      ringMesh.instanceMatrix.needsUpdate = true;
      if (ringMesh.instanceColor) ringMesh.instanceColor.needsUpdate = true;
    },
  };
}
