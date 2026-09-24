import * as THREE from 'three';

import type { NodeRec } from '../sim/bridge';
import { PALETTE, TREE } from './palette';
import type { FogView } from './fogView';
import { heightSampler } from './height';

// Harvestable nodes rendered as instanced meshes. Wood nodes render per
// tree species so the species reads before a chop starts; palms (food
// nodes on the coast) get their own silhouette. Oil stays hidden
// (Modern era feature). Visual scale tracks remaining amount; fog of
// war hides nodes in unexplored ground.

const KIND_WOOD = 1;
const KIND_STONE = 2;
const KIND_FOOD = 3;
const KIND_GOLD = 4;

export interface NodeObjects {
  group: THREE.Group;
  update(nodes: NodeRec[]): void;
  setFog(fog: FogView | null): void;
}

interface Part {
  geo: THREE.BufferGeometry;
  color: number;
}

// speciesParts returns the drawing parts for one wood species (and the
// palm), parts painted trunk-then-canopy.
function speciesParts(treeId: number): { parts: Part[]; offsetY: number } {
  switch (treeId) {
    case TREE.Birch: {
      const trunk = new THREE.CylinderGeometry(0.07, 0.1, 0.7, 5);
      trunk.translate(0, 0.35, 0);
      const crown = new THREE.IcosahedronGeometry(0.42, 0);
      crown.translate(0, 0.95, 0);
      return { parts: [{ geo: trunk, color: PALETTE.birchTrunk }, { geo: crown, color: PALETTE.birchLeaf }], offsetY: 0 };
    }
    case TREE.Baobab: {
      const trunk = new THREE.CylinderGeometry(0.28, 0.36, 0.75, 7);
      trunk.translate(0, 0.37, 0);
      const crown = new THREE.IcosahedronGeometry(0.5, 0);
      crown.scale(1.15, 0.55, 1.15);
      crown.translate(0, 0.95, 0);
      return { parts: [{ geo: trunk, color: PALETTE.baobabTrunk }, { geo: crown, color: PALETTE.baobabLeaf }], offsetY: 0 };
    }
    case TREE.Palm: {
      const trunk = new THREE.CylinderGeometry(0.07, 0.1, 0.9, 5);
      trunk.translate(0, 0.45, 0);
      const crown = new THREE.ConeGeometry(0.55, 0.3, 6);
      crown.scale(1, 0.55, 1);
      crown.translate(0, 0.98, 0);
      return { parts: [{ geo: trunk, color: PALETTE.palmTrunk }, { geo: crown, color: PALETTE.palmLeaf }], offsetY: 0 };
    }
    default: {
      // spruce: the default tree silhouette
      const trunk = new THREE.CylinderGeometry(0.09, 0.14, 0.5, 5);
      trunk.translate(0, 0.25, 0);
      const crown = new THREE.ConeGeometry(0.5, 1.2, 6);
      crown.translate(0, 1.05, 0);
      return { parts: [{ geo: trunk, color: PALETTE.trunk }, { geo: crown, color: PALETTE.spruceLeaf }], offsetY: 0 };
    }
  }
}

// stone/gold/berry keep their phase-3 props; berries (tree 0 food) and
// ore use a single low poly shape.
function plainParts(kind: number): { parts: Part[]; offsetY: number } {
  switch (kind) {
    case KIND_STONE:
      return { parts: [{ geo: new THREE.IcosahedronGeometry(0.42, 0), color: PALETTE.stone }], offsetY: 0.15 };
    case KIND_GOLD:
      return { parts: [{ geo: new THREE.IcosahedronGeometry(0.36, 0), color: PALETTE.gold }], offsetY: 0.12 };
    default:
      return { parts: [{ geo: new THREE.IcosahedronGeometry(0.3, 0), color: PALETTE.berry }], offsetY: 0.18 };
  }
}

// layerKey buckets nodes into one instanced draw group.
function layerKey(n: NodeRec): string {
  if (n.kind === KIND_WOOD) return `wood${n.tree}`;
  if (n.kind === KIND_FOOD && n.tree === TREE.Palm) return 'palm';
  return `k${n.kind}`;
}

export function buildNodeObjects(
  nodes: NodeRec[],
  size: number,
  heights: Uint16Array,
): NodeObjects {
  const group = new THREE.Group();
  const heightAt = heightSampler(size, heights);
  const cap = new Map<number, number>();
  for (const n of nodes) cap.set(n.id, n.amount);

  const lists = new Map<string, NodeRec[]>();
  for (const n of nodes) {
    if (n.kind === 5) continue; // oil hidden until the Modern era
    const key = layerKey(n);
    const list = lists.get(key);
    if (list) list.push(n);
    else lists.set(key, [n]);
  }

  interface Layer {
    meshes: THREE.InstancedMesh[];
    items: NodeRec[];
    ratio: number[];
    dep: boolean[];
    hidden: boolean[];
    origin: THREE.Vector3[];
    quat: THREE.Quaternion[];
    wood: boolean;
    base: THREE.Color[];
  }
  const layers: Layer[] = [];
  const matrix = new THREE.Matrix4();
  const euler = new THREE.Euler();
  const one = new THREE.Vector3(1, 1, 1);
  const scalePart = new THREE.Vector3();
  const colorScratch = new THREE.Color();

  for (const [key, items] of lists) {
    const spec = key.startsWith('wood')
      ? speciesParts(Number(key.slice(4)) || TREE.Spruce)
      : key === 'palm'
        ? speciesParts(TREE.Palm)
        : plainParts(key === 'k2' ? KIND_STONE : key === 'k4' ? KIND_GOLD : KIND_FOOD);
    const meshes = spec.parts.map((p) => {
      const mesh = new THREE.InstancedMesh(
        p.geo.toNonIndexed(),
        new THREE.MeshLambertMaterial({ color: 0xffffff, flatShading: true }),
        items.length,
      );
      mesh.castShadow = true;
      return mesh;
    });
    const origin: THREE.Vector3[] = [];
    const quats: THREE.Quaternion[] = [];
    items.forEach((n, i) => {
      const y = heightAt(n.x, n.y) + spec.offsetY;
      euler.set(0, (n.id % 628) / 100, 0);
      const q = new THREE.Quaternion().setFromEuler(euler);
      origin.push(new THREE.Vector3(n.x, y, n.y));
      quats.push(q);
      matrix.compose(origin[i], q, one);
      for (const m of meshes) m.setMatrixAt(i, matrix);
    });
    for (const m of meshes) group.add(m);
    layers.push({
      meshes,
      items,
      ratio: items.map(() => 1),
      dep: items.map(() => false),
      hidden: items.map(() => false),
      origin,
      quat: quats,
      wood: key.startsWith('wood') || key === 'palm',
      base: spec.parts.map((p) => new THREE.Color(p.color)),
    });
    // bake instance colors so brightness can track depletion
    for (let mi = 0; mi < meshes.length; mi++) {
      for (let i = 0; i < items.length; i++) meshes[mi].setColorAt(i, layers[layers.length - 1].base[mi]);
    }
  }

  const write = (layer: Layer, i: number): void => {
    const base = layer.base;
    for (let mi = 0; mi < layer.meshes.length; mi++) {
      const m = layer.meshes[mi];
      let s: THREE.Vector3;
      let bright: number;
      if (layer.hidden[i]) {
        s = new THREE.Vector3(0.0001, 0.0001, 0.0001);
        bright = 1;
      } else if (layer.dep[i]) {
        if (layer.wood) {
          // stump: keep a low trunk, drop the crown
          s = mi === 0 ? new THREE.Vector3(0.9, 0.16, 0.9) : new THREE.Vector3(0.0001, 0.0001, 0.0001);
        } else {
          s = new THREE.Vector3(0.22, 0.22, 0.22);
        }
        bright = 0.45;
      } else {
        const r = Math.max(0.001, layer.ratio[i]);
        s = new THREE.Vector3(r, r, r);
        bright = 0.62 + 0.38 * Math.min(1, Math.max(0, layer.ratio[i]));
      }
      scalePart.copy(s);
      matrix.compose(layer.origin[i], layer.quat[i], scalePart);
      m.setMatrixAt(i, matrix);
      colorScratch.copy(base[mi]).multiplyScalar(bright);
      m.setColorAt(i, colorScratch);
    }
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
        for (const m of layer.meshes) {
          m.instanceMatrix.needsUpdate = true;
          if (m.instanceColor) m.instanceColor.needsUpdate = true;
        }
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
        if (r === undefined) return false;
        const d = r <= 0;
        if (d === layer.dep[i] && (d || Math.abs(r - layer.ratio[i]) < 0.004)) return false;
        layer.dep[i] = d;
        if (!d) layer.ratio[i] = r;
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
