import * as THREE from 'three';

import type { BuildingDef, BuildingRec } from '../sim/bridge';

// RenderUnit is a display-ready unit: interpolated position, ground
// height y and a vertical bob offset for the walking animation.
export interface RenderUnit {
  id: number;
  owner: number;
  x: number;
  y: number;
  yOff: number;
  carried: number;
}
import { HEIGHT_SCALE, PALETTE, PLAYER_COLORS } from './palette';

// Dynamic entities: villagers as owner-colored instanced bodies, buildings
// as owner-colored boxes (sites grow with progress), plus selection rings.

const UNIT_CAP = 512;
const BUILDING_CAP = 256;

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
  const groundY = (tx: number, ty: number): number =>
    heights[ty * (size + 1) + tx] * HEIGHT_SCALE;
  const heightAt = (x: number, y: number): number => {
    const tx = Math.min(size - 1, Math.max(0, Math.floor(x)));
    const ty = Math.min(size - 1, Math.max(0, Math.floor(y)));
    return groundY(tx, ty);
  };

  const bodyGeo = new THREE.CapsuleGeometry(0.16, 0.34, 2, 6);
  bodyGeo.translate(0, 0.33, 0);
  const bodyMat = new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true });
  const unitMesh = new THREE.InstancedMesh(bodyGeo, bodyMat, UNIT_CAP);
  unitMesh.castShadow = true;
  unitMesh.count = 0;
  group.add(unitMesh);

  const boxGeo = new THREE.BoxGeometry(1, 1, 1);
  boxGeo.translate(0.5, 0, 0.5);
  const buildMat = new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true });
  const buildMesh = new THREE.InstancedMesh(boxGeo, buildMat, BUILDING_CAP);
  buildMesh.castShadow = true;
  buildMesh.count = 0;
  group.add(buildMesh);

  const ringGeo = new THREE.CylinderGeometry(0.42, 0.42, 0.05, 16);
  const ringMat = new THREE.MeshBasicMaterial({
    color: PALETTE.select,
    transparent: true,
    opacity: 0.55,
  });
  const rings = new THREE.InstancedMesh(ringGeo, ringMat, 64);
  rings.visible = false;
  group.add(rings);

  const matrix = new THREE.Matrix4();
  const quat = new THREE.Quaternion();
  const scale = new THREE.Vector3();
  const color = new THREE.Color();

  const unitPos = new Map<number, THREE.Vector3>();
  const buildPos = new Map<number, THREE.Vector3>();

  return {
    group,
    setUnits(units: RenderUnit[]): void {
      unitPos.clear();
      const n = Math.min(units.length, UNIT_CAP);
      for (let i = 0; i < n; i++) {
        const u = units[i];
        const p = new THREE.Vector3(u.x, heightAt(u.x, u.y) + u.yOff, u.y);
        unitPos.set(u.id, p);
        const s = u.carried > 0 ? 1.15 : 1;
        matrix.compose(p, quat, scale.set(s, s, s));
        unitMesh.setMatrixAt(i, matrix);
        color.setHex(PLAYER_COLORS[u.owner % PLAYER_COLORS.length]);
        unitMesh.setColorAt(i, color);
      }
      unitMesh.count = n;
      unitMesh.instanceMatrix.needsUpdate = true;
      if (unitMesh.instanceColor) unitMesh.instanceColor.needsUpdate = true;
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
        const p = new THREE.Vector3(b.tileX, groundY(b.tileX, b.tileY) + 0.02, b.tileY);
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
