import * as THREE from 'three';

import { hash01 } from './hash';
import { stepYaw, walkAmp, walkPhase } from './animate';
import { unitModel, UNIT_DEFS, type AnimPart } from './models';
import { PALETTE, RESOURCE_COLORS } from './palette';
import { heightSampler } from './height';
import { conditionGlyph, CONDITION_GLYPHS, emojiTexture } from './emoji';

// Dynamic units as instanced rigid-part models: a merged body, one
// player-tinted insignia mesh and up to five animated parts (legs, arm,
// weapon, carry), each pivoting with one matrix multiply per frame.
// Facing, walk cycle and attack cycles are derived from the interpolated
// positions the main loop already computes; no allocation per instance.

export interface RenderUnit {
  id: number;
  owner: number;
  x: number;
  y: number;
  carried: number;
  carriedKind: number;
  def: number;
  cls: number;
  state: number;
  target: number;
  // heading is the simulation compass direction in whole degrees, 0 at
  // +Y growing clockwise; the sim owns facing, the renderer only
  // smooths it.
  heading: number;
  // cond is the simulation condition bitmask (sim/core/condition.go).
  cond: number;
  color: number;
  hpFrac: number;
}

const UNIT_CAP = 1200;
const BAR_CAP = 300;
const RING_CAP = 96;
const GLYPH_CAP = 200;

// sim unit state bytes (mirrored from sim/core/entities.go)
export const STATE_GATHER = 2;
export const STATE_BUILD = 4;
export const STATE_ATTACK = 5;
export const STATE_SLEEP = 9;

interface Rig {
  body: THREE.InstancedMesh;
  insignia: THREE.InstancedMesh | null;
  anim: { part: AnimPart; mesh: THREE.InstancedMesh }[];
}

interface AnimState {
  yaw: number;
  walk: number;
  amp: number;
  cyc: number;
  state: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
}

export interface Actors {
  group: THREE.Group;
  setUnits(units: RenderUnit[], now: number): void;
  setSelection(ids: number[]): void;
  entityPosition(id: number, out: THREE.Vector3): boolean;
}

export function buildActors(size: number, heights: Uint16Array, camera: THREE.Camera): Actors {
  const group = new THREE.Group();
  const heightAt = heightSampler(size, heights);

  const lambert = () =>
    new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true, vertexColors: true });

  const rigs = new Map<number, Rig>();
  for (const def of UNIT_DEFS) {
    const model = unitModel(def);
    const body = new THREE.InstancedMesh(model.body, lambert(), UNIT_CAP);
    body.castShadow = true;
    body.frustumCulled = false;
    body.count = 0;
    group.add(body);
    let insignia: THREE.InstancedMesh | null = null;
    if (model.insignia) {
      insignia = new THREE.InstancedMesh(model.insignia, lambert(), UNIT_CAP);
      insignia.castShadow = true;
      insignia.frustumCulled = false;
      insignia.count = 0;
      group.add(insignia);
    }
    const anim: { part: AnimPart; mesh: THREE.InstancedMesh }[] = [];
    for (const part of model.anim) {
      const mesh = new THREE.InstancedMesh(part.geo, lambert(), UNIT_CAP);
      mesh.castShadow = true;
      mesh.frustumCulled = false;
      mesh.count = 0;
      group.add(mesh);
      anim.push({ part, mesh });
    }
    rigs.set(def, { body, insignia, anim });
  }

  // contact shadows: dark ground blobs under every unit so they read at
  // any zoom, before silhouettes are only a few pixels tall
  const blobGeo = new THREE.CircleGeometry(0.36, 14);
  blobGeo.rotateX(-Math.PI / 2);
  const blobMat = new THREE.MeshBasicMaterial({
    color: 0x141d10,
    transparent: true,
    opacity: 0.38,
    depthWrite: false,
  });
  const blobs = new THREE.InstancedMesh(blobGeo, blobMat, UNIT_CAP);
  blobs.frustumCulled = false;
  blobs.count = 0;
  group.add(blobs);

  // selection rings: flat torus with a shared pulse
  const ringGeo = new THREE.TorusGeometry(0.38, 0.045, 6, 28);
  ringGeo.rotateX(-Math.PI / 2);
  const ringMat = new THREE.MeshBasicMaterial({
    color: PALETTE.select,
    transparent: true,
    opacity: 0.6,
  });
  const rings = new THREE.InstancedMesh(ringGeo, ringMat, RING_CAP);
  rings.visible = false;
  rings.frustumCulled = false;
  group.add(rings);

  // hp bars over damaged units, billboarded to the camera
  const barBackGeo = new THREE.BoxGeometry(1, 0.075, 0.01);
  const barBack = new THREE.InstancedMesh(barBackGeo, new THREE.MeshBasicMaterial({ color: 0x11151f }), BAR_CAP);
  barBack.frustumCulled = false;
  barBack.count = 0;
  group.add(barBack);
  const barFillGeo = new THREE.BoxGeometry(1, 0.05, 0.014);
  const barFill = new THREE.InstancedMesh(barFillGeo, new THREE.MeshBasicMaterial({ color: 0xffffff }), BAR_CAP);
  barFill.frustumCulled = false;
  barFill.count = 0;
  group.add(barFill);

  // condition glyphs, billboarded over the hp bar: one instanced set per state
  const glyphGeo = new THREE.PlaneGeometry(1, 1);
  const glyphMeshes = CONDITION_GLYPHS.map((c) => {
    const mesh = new THREE.InstancedMesh(
      glyphGeo,
      new THREE.MeshBasicMaterial({ map: emojiTexture(c.glyph), transparent: true, depthWrite: false }),
      GLYPH_CAP,
    );
    mesh.frustumCulled = false;
    mesh.count = 0;
    mesh.renderOrder = 3;
    group.add(mesh);
    return mesh;
  });

  const matrix = new THREE.Matrix4();
  const partM = new THREE.Matrix4();
  const pivotM = new THREE.Matrix4();
  const spinM = new THREE.Matrix4();
  const baseM = new THREE.Matrix4();
  const quat = new THREE.Quaternion();
  const IDENTITY_Q = new THREE.Quaternion();
  const scaleV = new THREE.Vector3();
  const posV = new THREE.Vector3();
  const YAXIS = new THREE.Vector3(0, 1, 0);
  const XAXIS = new THREE.Vector3(1, 0, 0);
  const scratchColor = new THREE.Color();
  const GREEN = new THREE.Color(0x7bd86a);
  const RED = new THREE.Color(0xd86a6a);
  const glyphPos = new THREE.Vector3();
  const glyphScale = new THREE.Vector3();

  const animById = new Map<number, AnimState>();
  const unitPos = new Map<number, THREE.Vector3>();

  let lastNow = 0;

  function tint(hex: number, hpFrac: number): THREE.Color {
    scratchColor.setHex(hex);
    scratchColor.multiplyScalar(0.5 + 0.5 * Math.min(1, Math.max(0, hpFrac)));
    return scratchColor;
  }

  return {
    group,
    setUnits(units: RenderUnit[], now: number): void {
      const dt = lastNow === 0 ? 0.016 : Math.min(0.1, (now - lastNow) / 1000);
      lastNow = now;

      const counts = new Map<number, number>();
      unitPos.clear();
      const glyphN = CONDITION_GLYPHS.map(() => 0);
      let barN = 0;
      let blobN = 0;
      const camQuat = camera.quaternion;
      const right = new THREE.Vector3(1, 0, 0).applyQuaternion(camQuat);

      for (const u of units) {
        const rig = rigs.get(u.def);
        if (!rig) continue;
        const idx = counts.get(u.def) ?? 0;
        if (idx >= UNIT_CAP) continue;
        counts.set(u.def, idx + 1);

        let st = animById.get(u.id);
        if (!st) {
          st = { yaw: 0, walk: 0, amp: 0, cyc: 0, state: u.state, x: u.x, y: u.y, vx: 0, vy: 0 };
          animById.set(u.id, st);
        }
        const busyState = u.state === STATE_ATTACK || u.state === STATE_GATHER || u.state === STATE_BUILD;
        if (u.state !== st.state) {
          st.cyc = busyState ? 0.001 : 0;
          st.state = u.state;
        } else if (busyState) {
          st.cyc += dt;
        } else {
          st.cyc = 0;
        }

        const dx = u.x - st.x;
        const dy = u.y - st.y;
        const moved = Math.sqrt(dx * dx + dy * dy);
        st.x = u.x;
        st.y = u.y;

        // Smoothed velocity: raw per-frame deltas are tiny and noisy
        // (20 Hz ticks, fixed-point positions), so facing the raw delta
        // makes models flicker on slow or diagonal walks.
        const vel = Math.max(dt, 0.004);
        const blend = Math.min(1, dt * 10);
        st.vx += (dx / vel - st.vx) * blend;
        st.vy += (dy / vel - st.vy) * blend;
        const speed = Math.hypot(st.vx, st.vy);

        const moving = speed > 0.35;
        st.amp = walkAmp(st.amp, moving ? speed : 0, dt);
        st.walk = walkPhase(st.walk, moved);

        st.yaw = stepYaw(st.yaw, (u.heading * Math.PI) / 180, dt);

        const jitter = 0.94 + 0.12 * hash01(u.id, u.owner, 31);
        const bob = Math.abs(Math.sin(st.walk * 2)) * 0.05 * st.amp;
        const py = heightAt(u.x, u.y);
        posV.set(u.x, py + bob, u.y);
        // grow distant units (and their blobs/rings) so actors keep a
        // readable footprint when the camera pulls away
        const camD = camera.position.distanceTo(posV);
        const far = THREE.MathUtils.clamp((camD - 26) / 84, 0, 1);
        const unitScale = jitter * (1 + 0.65 * far);
        // sleeping animals settle onto the ground: the flattened base
        // pose carries through to the leg parts
        const flat = u.state === STATE_SLEEP ? 0.42 : 1;
        quat.setFromAxisAngle(YAXIS, st.yaw);
        scaleV.set(unitScale, unitScale * flat, unitScale);
        baseM.compose(posV, quat, scaleV);

        unitPos.set(u.id, new THREE.Vector3(u.x, py, u.y));

        matrix.compose(
          new THREE.Vector3(u.x, py + 0.02, u.y),
          IDENTITY_Q,
          scaleV.set(unitScale * 1.15, 1, unitScale * 1.15),
        );
        blobs.setMatrixAt(blobN, matrix);
        blobN++;

        rig.body.setMatrixAt(idx, baseM);
        rig.body.setColorAt(idx, tint(0xffffff, u.hpFrac));
        if (rig.insignia) {
          rig.insignia.setMatrixAt(idx, baseM);
          rig.insignia.setColorAt(idx, tint(u.color, u.hpFrac));
        }

        for (const entry of rig.anim) {
          const role = entry.part.role;
          const pivot = entry.part.pivot;
          let tx = pivot.x;
          let ty = pivot.y;
          let tz = pivot.z;
          let angle = 0;
          let colorHex = 0xffffff;
          let scale = 1;

          if (role === 'weapon') {
            if (st.cyc > 0) {
              const s = Math.max(0, Math.sin(st.cyc * 7.5));
              switch (u.def) {
                case 2:
                case 4:
                  tz += 0.16 * s;
                  break;
                case 7:
                  angle = -0.5 - 1.6 * s;
                  break;
                case 3:
                case 8:
                  tz -= 0.1 * Math.max(0, Math.sin(st.cyc * 9));
                  break;
                default:
                  angle = -0.35 - 0.85 * s;
              }
            } else if (u.cls === 1) {
              angle = -0.9;
            } else if (u.cls === 5) {
              scale = 0.0001;
            }
            colorHex = u.cls === 1 ? 0xffffff : PALETTE.steel;
          } else if (role === 'carry') {
            if (u.carried <= 0) {
              scale = 0.0001;
            } else {
              colorHex = RESOURCE_COLORS[u.carriedKind] ?? PALETTE.stone;
            }
          } else if (role === 'armR') {
            if (u.state === STATE_GATHER || u.state === STATE_BUILD) {
              angle = st.cyc > 0 ? -0.5 - Math.abs(Math.sin(st.cyc * 6)) * 1.1 : 0;
            } else {
              angle = Math.sin(st.walk + Math.PI) * st.amp * 1.2;
            }
          } else {
            const front = role === 'legF' || role === 'legL';
            angle = Math.sin(st.walk + (front ? 0 : Math.PI)) * st.amp;
          }

          pivotM.makeTranslation(tx, ty, tz);
          spinM.makeRotationAxis(XAXIS, angle);
          partM.copy(pivotM).multiply(spinM);
          matrix.copy(baseM).multiply(partM);
          if (scale !== 1) {
            matrix.multiply(spinM.makeScale(scale, scale, scale));
          }
          entry.mesh.setMatrixAt(idx, matrix);
          entry.mesh.setColorAt(idx, tint(colorHex, u.hpFrac));
        }

        if (u.hpFrac < 0.98 && barN < BAR_CAP) {
          const w = 0.46 * unitScale;
          const byy = py + 0.92 * unitScale;
          matrix.compose(
            new THREE.Vector3(u.x - (right.x * w) / 2, byy, u.y - (right.z * w) / 2),
            camQuat,
            new THREE.Vector3(w, 1, 1),
          );
          barBack.setMatrixAt(barN, matrix);
          const fw = w * u.hpFrac;
          matrix.compose(
            new THREE.Vector3(
              u.x - (right.x * (w - fw)) / 2,
              byy,
              u.y - (right.z * (w - fw)) / 2,
            ),
            camQuat,
            new THREE.Vector3(fw, 1, 1),
          );
          barFill.setMatrixAt(barN, matrix);
          scratchColor.copy(GREEN);
          scratchColor.lerp(RED, 1 - u.hpFrac);
          barFill.setColorAt(barN, scratchColor);
          barN++;
        }

        // one glyph per unit, the highest priority condition it carries
        const cond = conditionGlyph(u.cond);
        if (cond >= 0 && glyphN[cond] < GLYPH_CAP) {
          const gs = 0.42 * unitScale;
          const rise = py + (1.16 + 0.06 * Math.sin(now * 0.004 + u.id)) * unitScale;
          glyphPos.set(u.x, rise, u.y);
          glyphScale.set(gs, gs, 1);
          matrix.compose(glyphPos, camQuat, glyphScale);
          glyphMeshes[cond].setMatrixAt(glyphN[cond], matrix);
          glyphN[cond]++;
        }
      }

      for (const [def, rig] of rigs) {
        const n = counts.get(def) ?? 0;
        rig.body.count = n;
        rig.body.instanceMatrix.needsUpdate = true;
        if (rig.body.instanceColor) rig.body.instanceColor.needsUpdate = true;
        if (rig.insignia) {
          rig.insignia.count = n;
          rig.insignia.instanceMatrix.needsUpdate = true;
          if (rig.insignia.instanceColor) rig.insignia.instanceColor.needsUpdate = true;
        }
        for (const entry of rig.anim) {
          entry.mesh.count = n;
          entry.mesh.instanceMatrix.needsUpdate = true;
          if (entry.mesh.instanceColor) entry.mesh.instanceColor.needsUpdate = true;
        }
      }

      CONDITION_GLYPHS.forEach((_, i) => {
        glyphMeshes[i].count = glyphN[i];
        glyphMeshes[i].instanceMatrix.needsUpdate = true;
      });
      barBack.count = barN;
      barBack.instanceMatrix.needsUpdate = true;
      barFill.count = barN;
      barFill.instanceMatrix.needsUpdate = true;
      if (barFill.instanceColor) barFill.instanceColor.needsUpdate = true;
      blobs.count = blobN;
      blobs.instanceMatrix.needsUpdate = true;

      if (animById.size > units.length * 2 + 256) {
        const seen = new Set(units.map((u) => u.id));
        for (const id of animById.keys()) if (!seen.has(id)) animById.delete(id);
      }
    },
    setSelection(ids: number[]): void {
      const pulse = 0.45 + 0.25 * (0.5 + 0.5 * Math.sin(performance.now() * 0.006));
      ringMat.opacity = pulse;
      const shown = ids.slice(0, RING_CAP);
      let n = 0;
      for (const id of shown) {
        const p = unitPos.get(id);
        if (!p) continue;
        const camD = camera.position.distanceTo(p);
        const far = THREE.MathUtils.clamp((camD - 26) / 84, 0, 1);
        const s = 1 + 0.55 * far;
        matrix.compose(new THREE.Vector3(p.x, p.y + 0.03, p.z), IDENTITY_Q, scaleV.set(s, 1, s));
        rings.setMatrixAt(n, matrix);
        n++;
      }
      rings.count = n;
      rings.visible = n > 0;
      rings.instanceMatrix.needsUpdate = true;
    },
    entityPosition(id: number, out: THREE.Vector3): boolean {
      const p = unitPos.get(id);
      if (!p) return false;
      out.copy(p);
      return true;
    },
  };
}

