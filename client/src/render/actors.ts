import * as THREE from 'three';

import type { BuildingDef, BuildingRec } from '../sim/bridge';

// RenderUnit is a display-ready unit: interpolated position, ground
// height y, a vertical bob offset for the walking animation, the def for
// body shape, a base color and an hp fraction used for damage tint.
export interface RenderUnit {
  id: number;
  owner: number;
  x: number;
  y: number;
  yOff: number;
  carried: number;
  def: number;
  cls: number;
  color: number;
  hpFrac: number;
}
import { MOUNT_COLOR, PALETTE } from './palette';
import { heightSampler } from './height';

// Dynamic entities: humanoids as owner-colored instanced capsules, with
// spear tips for lancers and mounts for scouts, animals as low boxes,
// buildings as owner-colored boxes (sites grow with progress), plus
// selection rings.

const UNIT_CAP = 512;
const EXTRA_CAP = 128;
const BUILDING_CAP = 256;

// Unit class ids mirrored from sim/units.
const CLASS_SCOUT = 4;
const CLASS_ANIMAL = 5;
const DEF_LANCER = 2;

export interface Actors {
  group: THREE.Group;
  setUnits(units: RenderUnit[]): void;
  setBuildings(buildings: BuildingRec[], defs: Map<number, BuildingDef>): void;
  setSelection(ids: number[]): void;
  // screenPoint fills v with the projected position of entity id; false if gone.
  entityPosition(id: number, out: THREE.Vector3): boolean;
}

export function buildActors(size: number, heights: Uint16Array): Actors {
  const group = new THREE.Group();
  const heightAt = heightSampler(size, heights);

  const bodyGeo = new THREE.CapsuleGeometry(0.16, 0.34, 2, 6);
  bodyGeo.translate(0, 0.33, 0);
  const bodyMat = new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true });
  const unitMesh = new THREE.InstancedMesh(bodyGeo, bodyMat, UNIT_CAP);
  unitMesh.castShadow = true;
  unitMesh.count = 0;
  // Instance matrices move every frame and three caches the bounding sphere
  // on first use; culling would keep the mesh invisible forever.
  unitMesh.frustumCulled = false;
  group.add(unitMesh);

  const boxGeo = new THREE.BoxGeometry(1, 1, 1);
  boxGeo.translate(0.5, 0, 0.5);
  const buildMat = new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true });
  const buildMesh = new THREE.InstancedMesh(boxGeo, buildMat, BUILDING_CAP);
  buildMesh.castShadow = true;
  buildMesh.count = 0;
  buildMesh.frustumCulled = false;
  group.add(buildMesh);

  const ringGeo = new THREE.CylinderGeometry(0.42, 0.42, 0.05, 16);
  const ringMat = new THREE.MeshBasicMaterial({
    color: PALETTE.select,
    transparent: true,
    opacity: 0.55,
  });
  const rings = new THREE.InstancedMesh(ringGeo, ringMat, 64);
  rings.visible = false;
  rings.frustumCulled = false;
  group.add(rings);

  const spearGeo = new THREE.ConeGeometry(0.05, 0.55, 5);
  spearGeo.translate(0, 0.62, 0.12);
  const spearMat = new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true });
  const spearMesh = new THREE.InstancedMesh(spearGeo, spearMat, EXTRA_CAP);
  spearMesh.count = 0;
  spearMesh.frustumCulled = false;
  group.add(spearMesh);

  const beastGeo = new THREE.BoxGeometry(0.34, 0.3, 0.62);
  beastGeo.translate(0, 0.18, 0);
  const beastMat = new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true });
  const beastMesh = new THREE.InstancedMesh(beastGeo, beastMat, EXTRA_CAP);
  beastMesh.count = 0;
  beastMesh.frustumCulled = false;
  group.add(beastMesh);

  const matrix = new THREE.Matrix4();
  const quat = new THREE.Quaternion();
  const scale = new THREE.Vector3();
  const color = new THREE.Color();

  const unitPos = new Map<number, THREE.Vector3>();
  const buildPos = new Map<number, THREE.Vector3>();

  const tinted = (hex: number, hpFrac: number): THREE.Color =>
    color.clone().setHex(hex).multiplyScalar(0.45 + 0.55 * Math.min(1, Math.max(0, hpFrac)));

  return {
    group,
    setUnits(units: RenderUnit[]): void {
      unitPos.clear();
      let n = 0;
      let ns = 0;
      let nb = 0;
      for (const u of units) {
        if (n >= UNIT_CAP) break;
        const p = new THREE.Vector3(u.x, heightAt(u.x, u.y) + u.yOff, u.y);
        unitPos.set(u.id, p);
        if (u.cls === CLASS_ANIMAL) {
          if (nb < EXTRA_CAP) {
            matrix.compose(p, quat, scale.set(1, 1, 1));
            beastMesh.setMatrixAt(nb, matrix);
            beastMesh.setColorAt(nb, tinted(u.color, u.hpFrac));
            nb++;
          }
          continue;
        }
        const s = u.carried > 0 ? 1.15 : u.cls === CLASS_SCOUT ? 1.12 : 1;
        matrix.compose(p, quat, scale.set(s, s, s));
        unitMesh.setMatrixAt(n, matrix);
        unitMesh.setColorAt(n, tinted(u.color, u.hpFrac));
        n++;
        if (u.cls === CLASS_SCOUT && nb < EXTRA_CAP) {
          const mp = new THREE.Vector3(u.x, heightAt(u.x, u.y) + u.yOff * 0.5, u.y);
          matrix.compose(mp, quat, scale.set(1.35, 0.75, 1.6));
          beastMesh.setMatrixAt(nb, matrix);
          beastMesh.setColorAt(nb, tinted(MOUNT_COLOR, u.hpFrac));
          nb++;
        }
        if (u.def === DEF_LANCER && ns < EXTRA_CAP) {
          matrix.compose(p, quat, scale.set(1, 1, 1));
          spearMesh.setMatrixAt(ns, matrix);
          spearMesh.setColorAt(ns, color.clone().setHex(0xc9c9c9));
          ns++;
        }
      }
      unitMesh.count = n;
      spearMesh.count = ns;
      beastMesh.count = nb;
      unitMesh.instanceMatrix.needsUpdate = true;
      if (unitMesh.instanceColor) unitMesh.instanceColor.needsUpdate = true;
      spearMesh.instanceMatrix.needsUpdate = true;
      if (spearMesh.instanceColor) spearMesh.instanceColor.needsUpdate = true;
      beastMesh.instanceMatrix.needsUpdate = true;
      if (beastMesh.instanceColor) beastMesh.instanceColor.needsUpdate = true;
    },
    setBuildings(buildings: BuildingRec[], defs: Map<number, BuildingDef>): void {
      buildPos.clear();
      const n = Math.min(buildings.length, BUILDING_CAP);
      for (let i = 0; i < n; i++) {
        const b = buildings[i];
        const def = defs.get(b.def);
        const sizeTiles = def ? def.size : 1;
        const done = def && def.buildTicks > 0 ? 1 - b.progress / def.buildTicks : 1;
        const h = 0.35 + 0.9 * done;
        const p = new THREE.Vector3(b.tileX, heightAt(b.tileX, b.tileY) + 0.02, b.tileY);
        buildPos.set(b.id, p);
        matrix.compose(p, quat, scale.set(sizeTiles, h, sizeTiles));
        buildMesh.setMatrixAt(i, matrix);
        const base =
          def && def.key === 'campfire'
            ? PALETTE.campfire
            : def && def.key === 'storehouse'
              ? PALETTE.storehouse
              : PALETTE.hut;
        color.setHex(base).multiplyScalar(0.6 + 0.4 * done);
        buildMesh.setColorAt(i, color);
      }
      buildMesh.count = n;
      buildMesh.instanceMatrix.needsUpdate = true;
      if (buildMesh.instanceColor) buildMesh.instanceColor.needsUpdate = true;
    },
    setSelection(ids: number[]): void {
      const shown = ids.slice(0, 64);
      shown.forEach((id, i) => {
        const isUnit = unitPos.has(id);
        const p = isUnit ? unitPos.get(id)! : buildPos.get(id)!;
        if (!p) return;
        const r = isUnit ? 1 : 1.6;
        const cx = isUnit ? p.x : p.x + 0.5;
        const cz = isUnit ? p.z : p.z + 0.5;
        matrix.compose(
          new THREE.Vector3(cx, p.y + 0.05, cz),
          quat,
          scale.set(r, 1, r),
        );
        rings.setMatrixAt(i, matrix);
      });
      rings.count = shown.length;
      rings.visible = shown.length > 0;
      rings.instanceMatrix.needsUpdate = true;
    },
    entityPosition(id: number, out: THREE.Vector3): boolean {
      const p = unitPos.get(id) ?? buildPos.get(id);
      if (!p) return false;
      out.copy(p);
      return true;
    },
  };
}
