import * as THREE from 'three';

import type { NodeRec } from '../sim/bridge';
import { HEIGHT_SCALE, PALETTE } from './palette';
import type { FogView } from './fogView';

// Harvestable nodes rendered as instanced meshes, one per resource kind.
// Oil is hidden entirely (Modern era feature). Visual scale tracks
// remaining amount; fog of war hides nodes in unexplored ground.

const KIND_WOOD = 1;
const KIND_STONE = 2;
const KIND_FOOD = 3;
const KIND_GOLD = 4;

export interface NodeObjects {
  group: THREE.Group;
  update(nodes: NodeRec[]): void;
  setFog(fog: FogView | null): void;
}

function lambert(color: number): THREE.MeshLambertMaterial {
  return new THREE.MeshLambertMaterial({ color, flatShading: true });
}

export function buildNodeObjects(
  nodes: NodeRec[],
  size: number,
  heights: Uint16Array,
): NodeObjects {
  const group = new THREE.Group();
  const cap = new Map<number, number>();
  for (const n of nodes) cap.set(n.id, n.amount);
  const groundY = (tx: number, ty: number): number =>
    heights[ty * (size + 1) + tx] * HEIGHT_SCALE;

  const kinds = [KIND_WOOD, KIND_STONE, KIND_GOLD, KIND_FOOD];
  const lists = new Map<number, NodeRec[]>();
  for (const k of kinds) lists.set(k, []);
  for (const n of nodes) {
    lists.get(n.kind)?.push(n);
  }

  const trunk = new THREE.CylinderGeometry(0.09, 0.14, 0.5, 5);
  trunk.translate(0, 0.25, 0);
  const cone = new THREE.ConeGeometry(0.5, 1.2, 6);
  cone.translate(0, 1.05, 0);
  const treeBody = trunk.toNonIndexed();
  trunk.dispose();

  const geos: Record<number, THREE.BufferGeometry> = {
    [KIND_WOOD]: treeBody,
    [KIND_STONE]: new THREE.IcosahedronGeometry(0.42, 0),
    [KIND_GOLD]: new THREE.IcosahedronGeometry(0.36, 0),
    [KIND_FOOD]: new THREE.IcosahedronGeometry(0.3, 0),
  };
  const colors: Record<number, number> = {
    [KIND_WOOD]: PALETTE.leaf,
    [KIND_STONE]: PALETTE.stone,
    [KIND_GOLD]: PALETTE.gold,
    [KIND_FOOD]: PALETTE.berry,
  };
  const yOffsets: Record<number, number> = {
    [KIND_WOOD]: 0,
    [KIND_STONE]: 0.15,
    [KIND_GOLD]: 0.12,
    [KIND_FOOD]: 0.18,
  };

  interface Layer {
    mesh: THREE.InstancedMesh;
    trunks: THREE.InstancedMesh | null;
    items: NodeRec[];
    ratio: number[];
    hidden: boolean[];
    origin: THREE.Vector3[];
    quat: THREE.Quaternion[];
  }
  const layers: Layer[] = [];
  const matrix = new THREE.Matrix4();
  const euler = new THREE.Euler();
  const one = new THREE.Vector3(1, 1, 1);

  for (const kind of kinds) {
    const items = lists.get(kind)!;
    if (items.length === 0) continue;
    const mesh = new THREE.InstancedMesh(geos[kind], lambert(colors[kind]), items.length);
    mesh.castShadow = true;
    let trunks: THREE.InstancedMesh | null = null;
    if (kind === KIND_WOOD) {
      trunks = new THREE.InstancedMesh(cone.toNonIndexed(), lambert(PALETTE.trunk), items.length);
      trunks.castShadow = true;
    }
    const origin: THREE.Vector3[] = [];
    const quats: THREE.Quaternion[] = [];
    items.forEach((n, i) => {
      const y = groundY(n.tileX, n.tileY) + yOffsets[kind];
      euler.set(0, (n.id % 628) / 100, 0);
      const q = new THREE.Quaternion().setFromEuler(euler);
      origin.push(new THREE.Vector3(n.x, y, n.y));
      quats.push(q);
      matrix.compose(origin[i], q, one);
      mesh.setMatrixAt(i, matrix);
      if (trunks) trunks.setMatrixAt(i, matrix);
    });
    group.add(mesh);
    if (trunks) group.add(trunks);
    layers.push({
      mesh,
      trunks,
      items,
      ratio: items.map(() => 1),
      hidden: items.map(() => false),
      origin,
      quat: quats,
    });
  }

  const write = (layer: Layer, i: number): void => {
    const s = layer.hidden[i] ? 0.0001 : Math.max(0.001, layer.ratio[i]);
    matrix.compose(layer.origin[i], layer.quat[i], new THREE.Vector3(s, s, s));
    layer.mesh.setMatrixAt(i, matrix);
    if (layer.trunks) layer.trunks.setMatrixAt(i, matrix);
  };

  const refresh = (predicate: (layer: Layer, i: number) => boolean): void => {
    for (const layer of layers) {
      let dirty = false;
      for (let i = 0; i < layer.items.length; i++) {
        if (predicate(layer, i)) {
          write(layer, i);
          dirty = true;
        }
      }
      if (dirty) {
        layer.mesh.instanceMatrix.needsUpdate = true;
        if (layer.trunks) layer.trunks.instanceMatrix.needsUpdate = true;
      }
    }
  };

  return {
    group,
    update(updates: NodeRec[]): void {
      const next = new Map<number, number>();
      for (const n of updates) {
        next.set(n.id, n.amount <= 0 ? 0 : Math.max(0.35, n.amount / Math.max(1, cap.get(n.id) ?? n.amount)));
      }
      refresh((layer, i) => {
        const r = next.get(layer.items[i].id);
        if (r === undefined || Math.abs(r - layer.ratio[i]) < 0.004) return false;
        layer.ratio[i] = r;
        return true;
      });
    },
    setFog(fog: FogView | null): void {
      refresh((layer, i) => {
        const n = layer.items[i];
        const hidden = fog !== null && !fog.visible(n.tileX, n.tileY);
        if (hidden === layer.hidden[i]) return false;
        layer.hidden[i] = hidden;
        return true;
      });
    },
  };
}
