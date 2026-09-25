import * as THREE from 'three';

import type { BuildingDef, BuildingRec } from '../sim/bridge';
import { buildingModel } from './models';
import { PALETTE, PLAYER_COLORS } from './palette';
import { heightSampler } from './height';

// Buildings render as code-generated silhouettes with a player-tinted
// insignia part. A building under construction shows a foundation slab,
// a rising player-colored core and scaffold poles; the full model swaps
// in when the site completes. Campfire flames flicker per frame.

const BUILD_CAP = 256;

export interface Buildings {
  group: THREE.Group;
  setBuildings(recs: BuildingRec[], defs: Map<number, BuildingDef>, now: number): void;
  setSelection(ids: number[]): void;
  entityPosition(id: number, out: THREE.Vector3): boolean;
}

export function buildBuildings(size: number, heights: Uint16Array): Buildings {
  const group = new THREE.Group();
  const heightAt = heightSampler(size, heights);

  const lambert = () =>
    new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true, vertexColors: true });

  interface DefRig {
    structure: THREE.InstancedMesh;
    insignia: THREE.InstancedMesh | null;
  }
  const rigs = new Map<number, DefRig>();

  const mat = new THREE.Matrix4();
  const one = new THREE.Vector3(1, 1, 1);
  const col = new THREE.Color();

  const buildPos = new Map<number, THREE.Vector3>();

  // construction scaffolding, shared across all sites
  const foundGeo = new THREE.BoxGeometry(1, 0.1, 1);
  foundGeo.translate(0, 0.05, 0);
  const foundMat = new THREE.MeshLambertMaterial({ color: PALETTE.stone, flatShading: true });
  const founds = new THREE.InstancedMesh(foundGeo, foundMat, BUILD_CAP);
  founds.frustumCulled = false;
  founds.count = 0;
  group.add(founds);

  const coreGeo = new THREE.BoxGeometry(1, 1, 1);
  coreGeo.translate(0, 0.5, 0);
  const coreMat = new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true });
  const cores = new THREE.InstancedMesh(coreGeo, coreMat, BUILD_CAP);
  cores.castShadow = true;
  cores.frustumCulled = false;
  cores.count = 0;
  group.add(cores);

  const scaffold = new THREE.Group();
  const poleGeo = new THREE.BoxGeometry(0.06, 1.1, 0.06);
  poleGeo.translate(0, 0.55, 0);
  const barGeo = new THREE.BoxGeometry(0.05, 0.05, 1.5);
  barGeo.translate(0, 0.85, 0);
  const scaffoldMat = new THREE.MeshLambertMaterial({ color: PALETTE.trunk, flatShading: true });
  const poles = new THREE.InstancedMesh(poleGeo, scaffoldMat, BUILD_CAP * 4);
  const bars = new THREE.InstancedMesh(barGeo, scaffoldMat, BUILD_CAP * 2);
  for (const m of [poles, bars]) {
    m.frustumCulled = false;
    m.count = 0;
    scaffold.add(m);
  }
  group.add(scaffold);

  // campfire flames
  const flameGeo = new THREE.ConeGeometry(0.13, 0.34, 5);
  flameGeo.translate(0, 0.28, 0);
  const flameMat = new THREE.MeshBasicMaterial({ color: PALETTE.flame });
  const flames = new THREE.InstancedMesh(flameGeo, flameMat, 128);
  flames.frustumCulled = false;
  flames.count = 0;
  group.add(flames);
  const emberGeo = new THREE.ConeGeometry(0.07, 0.2, 5);
  emberGeo.translate(0, 0.2, 0);
  const emberMat = new THREE.MeshBasicMaterial({ color: PALETTE.ember });
  const embers = new THREE.InstancedMesh(emberGeo, emberMat, 128);
  embers.frustumCulled = false;
  embers.count = 0;
  group.add(embers);

  // selection rings for buildings
  const ringGeo = new THREE.TorusGeometry(1.15, 0.05, 6, 32);
  ringGeo.rotateX(-Math.PI / 2);
  const ringMat = new THREE.MeshBasicMaterial({ color: PALETTE.select, transparent: true, opacity: 0.5 });
  const rings = new THREE.InstancedMesh(ringGeo, ringMat, 64);
  rings.visible = false;
  rings.frustumCulled = false;
  group.add(rings);

  return {
    group,
    setBuildings(recs: BuildingRec[], defs: Map<number, BuildingDef>, now: number): void {
      buildPos.clear();
      const used = new Map<number, number>();
      let nf = 0;
      let ncore = 0;
      let npole = 0;
      let nbar = 0;
      let nflame = 0;

      for (const b of recs) {
        if (used.size >= BUILD_CAP) break;
        const def = defs.get(b.def);
        const sizeTiles = def ? def.size : 1;
        const done = def && def.buildTicks > 0 ? 1 - b.progress / def.buildTicks : 1;
        const key = def ? def.key : 'hut';
        const cx = b.tileX + sizeTiles / 2;
        const cz = b.tileY + sizeTiles / 2;
        const gy = heightAt(b.tileX, b.tileY);
        buildPos.set(b.id, new THREE.Vector3(cx, gy, cz));

        const n = used.get(b.def) ?? 0;

        if (b.progress > 0) {
          // foundation + growing core + scaffold
          mat.compose(
            new THREE.Vector3(cx, gy, cz),
            new THREE.Quaternion(),
            new THREE.Vector3(sizeTiles, 1, sizeTiles),
          );
          if (nf < BUILD_CAP) {
            founds.setMatrixAt(nf, mat);
            nf++;
          }
          if (ncore < BUILD_CAP) {
            const hgt = Math.max(0.15, done * sizeTiles * 0.7);
            mat.compose(
              new THREE.Vector3(cx, gy + 0.08, cz),
              new THREE.Quaternion(),
              new THREE.Vector3(sizeTiles * 0.62, hgt, sizeTiles * 0.62),
            );
            cores.setMatrixAt(ncore, mat);
            cores.setColorAt(ncore, col.setHex(playerHex(b.owner)).multiplyScalar(0.7 + 0.3 * done));
            ncore++;
          }
          const half = sizeTiles / 2;
          for (const [sx, sz] of [
            [-half, -half],
            [half, -half],
            [-half, half],
            [half, half],
          ]) {
            if (npole >= BUILD_CAP * 4) break;
            mat.compose(
              new THREE.Vector3(cx + sx, gy, cz + sz),
              new THREE.Quaternion(),
              new THREE.Vector3(1, 0.6 + done * 0.6, 1),
            );
            poles.setMatrixAt(npole, mat);
            npole++;
          }
          if (nbar < BUILD_CAP * 2) {
            mat.compose(new THREE.Vector3(cx, gy + 0.9, cz - half), new THREE.Quaternion(), one);
            bars.setMatrixAt(nbar, mat);
            nbar++;
            mat.compose(new THREE.Vector3(cx, gy + 0.6, cz + half), new THREE.Quaternion(), one);
            bars.setMatrixAt(nbar, mat);
            nbar++;
          }
          used.set(b.def, n);
          continue;
        }

        let rig = rigs.get(b.def);
        if (!rig) {
          const model = buildingModel(key);
          const structure = new THREE.InstancedMesh(model.structure, lambert(), BUILD_CAP);
          structure.castShadow = true;
          structure.receiveShadow = true;
          structure.frustumCulled = false;
          group.add(structure);
          let insignia: THREE.InstancedMesh | null = null;
          if (model.insignia) {
            insignia = new THREE.InstancedMesh(model.insignia, lambert(), BUILD_CAP);
            insignia.frustumCulled = false;
            group.add(insignia);
          }
          rig = { structure, insignia };
          rigs.set(b.def, rig);
        }
        if (n >= BUILD_CAP) continue;
        mat.compose(new THREE.Vector3(cx, gy, cz), new THREE.Quaternion(), one);
        rig.structure.setMatrixAt(n, mat);
        rig.structure.setColorAt(n, col.setHex(0xffffff));
        if (rig.insignia) {
          rig.insignia.setMatrixAt(n, mat);
          rig.insignia.setColorAt(n, col.setHex(playerHex(b.owner)));
        }
        if (key === 'campfire' && nflame < 128) {
          const f = 0.75 + 0.35 * Math.sin(now * 0.011 + b.id * 1.7);
          const g = 0.8 + 0.3 * Math.sin(now * 0.017 + b.id * 2.3);
          mat.compose(new THREE.Vector3(cx, gy, cz), new THREE.Quaternion(), new THREE.Vector3(f, g, f));
          flames.setMatrixAt(nflame, mat);
          mat.compose(new THREE.Vector3(cx, gy, cz), new THREE.Quaternion(), new THREE.Vector3(g, f, g));
          embers.setMatrixAt(nflame, mat);
          nflame++;
        }
        used.set(b.def, n + 1);
      }

      founds.count = nf;
      founds.instanceMatrix.needsUpdate = true;
      cores.count = ncore;
      cores.instanceMatrix.needsUpdate = true;
      if (cores.instanceColor) cores.instanceColor.needsUpdate = true;
      poles.count = npole;
      poles.instanceMatrix.needsUpdate = true;
      bars.count = nbar;
      bars.instanceMatrix.needsUpdate = true;
      flames.count = nflame;
      flames.instanceMatrix.needsUpdate = true;
      embers.count = nflame;
      embers.instanceMatrix.needsUpdate = true;

      for (const [def, rig] of rigs) {
        const n = used.get(def) ?? 0;
        if (rig.structure.count !== n || n > 0) {
          rig.structure.count = n;
          rig.structure.instanceMatrix.needsUpdate = true;
          if (rig.structure.instanceColor) rig.structure.instanceColor.needsUpdate = true;
        }
        if (rig.insignia) {
          rig.insignia.count = n;
          rig.insignia.instanceMatrix.needsUpdate = true;
          if (rig.insignia.instanceColor) rig.insignia.instanceColor.needsUpdate = true;
        }
      }
    },
    setSelection(ids: number[]): void {
      let n = 0;
      for (const id of ids.slice(0, 64)) {
        const p = buildPos.get(id);
        if (!p) continue;
        mat.compose(new THREE.Vector3(p.x, p.y + 0.04, p.z), new THREE.Quaternion(), one);
        rings.setMatrixAt(n, mat);
        n++;
      }
      rings.count = n;
      rings.visible = n > 0;
      rings.instanceMatrix.needsUpdate = true;
    },
    entityPosition(id: number, out: THREE.Vector3): boolean {
      const p = buildPos.get(id);
      if (!p) return false;
      out.copy(p);
      return true;
    },
  };
}

// Player body colors indexed by owner; neutral owners get stone.
function playerHex(owner: number): number {
  if (owner >= PLAYER_COLORS.length) return PALETTE.stone;
  return PLAYER_COLORS[owner];
}
