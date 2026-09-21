import * as THREE from 'three';

import { HEIGHT_SCALE, PALETTE, TERRAIN } from './palette';

// Low-poly map objects drawn with one InstancedMesh per kind: trees on
// forest tiles, rocks on rock tiles, gold discs on spawn points. Each
// instance carries a deterministic rotation and scale jitter from a local
// PRNG — cosmetic only, never simulation state.

export interface MapFeature {
  // id is the tile index, stable for the map.
  id: number;
  position: THREE.Vector3;
}

export interface MapObjects {
  group: THREE.Group;
  features: MapFeature[];
  setSelection(ids: number[]): void;
}

function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function paint(geometry: THREE.BufferGeometry, hex: number): THREE.BufferGeometry {
  const g = geometry.toNonIndexed();
  const count = g.getAttribute('position').count;
  const colors = new Float32Array(count * 3);
  const color = new THREE.Color(hex);
  for (let i = 0; i < count; i++) {
    colors[i * 3] = color.r;
    colors[i * 3 + 1] = color.g;
    colors[i * 3 + 2] = color.b;
  }
  g.setAttribute('color', new THREE.BufferAttribute(colors, 3));
  return g;
}

// mergeConcat appends non-indexed geometries that share position/normal/color.
function mergeConcat(parts: THREE.BufferGeometry[]): THREE.BufferGeometry {
  const merged = new THREE.BufferGeometry();
  const attrs = ['position', 'normal', 'color'] as const;
  for (const name of attrs) {
    const total = parts.reduce((n, p) => n + p.getAttribute(name).count, 0);
    const itemSize = parts[0].getAttribute(name).itemSize;
    const arr = new Float32Array(total * itemSize);
    let offset = 0;
    for (const p of parts) {
      arr.set(p.getAttribute(name).array as Float32Array, offset);
      offset += p.getAttribute(name).count * itemSize;
    }
    merged.setAttribute(name, new THREE.BufferAttribute(arr, itemSize));
  }
  return merged;
}

function treeGeometry(): THREE.BufferGeometry {
  const trunk = new THREE.CylinderGeometry(0.09, 0.14, 0.5, 5);
  trunk.translate(0, 0.25, 0);
  const cone = new THREE.ConeGeometry(0.5, 1.2, 6);
  cone.translate(0, 1.05, 0);
  return mergeConcat([paint(trunk, PALETTE.trunk), paint(cone, PALETTE.leaf)]);
}

export function buildMapObjects(
  size: number,
  tiles: Uint8Array,
  heights: Uint16Array,
  spawns: { x: number; y: number }[],
  prngSeed: number,
): MapObjects {
  const rand = mulberry32(prngSeed);
  const group = new THREE.Group();
  const features: MapFeature[] = [];

  const groundY = (tx: number, ty: number): number =>
    heights[ty * (size + 1) + tx] * HEIGHT_SCALE;

  const forestTiles: number[] = [];
  const rockTiles: number[] = [];
  for (let i = 0; i < tiles.length; i++) {
    if (tiles[i] === TERRAIN.Forest) forestTiles.push(i);
    if (tiles[i] === TERRAIN.Rock) rockTiles.push(i);
  }

  const treeGeo = treeGeometry();
  const treeMat = new THREE.MeshLambertMaterial({ vertexColors: true, flatShading: true });
  const trees = new THREE.InstancedMesh(treeGeo, treeMat, forestTiles.length);
  trees.castShadow = true;

  const rockGeo = new THREE.IcosahedronGeometry(0.42, 0);
  const rockMat = new THREE.MeshLambertMaterial({ color: PALETTE.stone, flatShading: true });
  const rocks = new THREE.InstancedMesh(rockGeo, rockMat, rockTiles.length);
  rocks.castShadow = true;

  const markerGeo = new THREE.CylinderGeometry(1.1, 1.1, 0.12, 8);
  const markerMat = new THREE.MeshLambertMaterial({ color: PALETTE.marker, flatShading: true });
  const markers = new THREE.InstancedMesh(markerGeo, markerMat, Math.max(spawns.length, 1));

  const matrix = new THREE.Matrix4();
  const quat = new THREE.Quaternion();
  const euler = new THREE.Euler();
  const scale = new THREE.Vector3();

  const place = (mesh: THREE.InstancedMesh, index: number, x: number, z: number, y: number): void => {
    quat.setFromEuler(euler.set(0, rand() * Math.PI * 2, 0));
    const s = 0.8 + rand() * 0.5;
    scale.set(s, s, s);
    matrix.compose(new THREE.Vector3(x, y, z), quat, scale);
    mesh.setMatrixAt(index, matrix);
  };

  forestTiles.forEach((tile, i) => {
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    const x = tx + 0.5;
    const z = ty + 0.5;
    place(trees, i, x, z, groundY(tx, ty));
    features.push({ id: tile, position: new THREE.Vector3(x, groundY(tx, ty), z) });
  });

  rockTiles.forEach((tile, i) => {
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    const x = tx + 0.5;
    const z = ty + 0.5;
    place(rocks, i, x, z, groundY(tx, ty) + 0.15);
    features.push({ id: tile, position: new THREE.Vector3(x, groundY(tx, ty), z) });
  });

  spawns.forEach((s, i) => {
    const y = groundY(s.x, s.y) + 0.06;
    matrix.compose(new THREE.Vector3(s.x + 0.5, y, s.y + 0.5), new THREE.Quaternion(), new THREE.Vector3(1, 1, 1));
    markers.setMatrixAt(i, matrix);
    features.push({ id: s.y * size + s.x, position: new THREE.Vector3(s.x + 0.5, y, s.y + 0.5) });
  });

  trees.instanceMatrix.needsUpdate = true;
  rocks.instanceMatrix.needsUpdate = true;
  markers.instanceMatrix.needsUpdate = true;
  group.add(trees, rocks, markers);

  // Selection rings: flat white discs drawn under the selected features.
  const ringGeo = new THREE.CylinderGeometry(1.15, 1.15, 0.05, 20);
  const ringMat = new THREE.MeshBasicMaterial({
    color: PALETTE.select,
    transparent: true,
    opacity: 0.55,
  });
  const rings = new THREE.InstancedMesh(ringGeo, ringMat, 64);
  rings.visible = false;
  group.add(rings);

  const byId = new Map<number, MapFeature>();
  for (const f of features) byId.set(f.id, f);

  return {
    group,
    features,
    setSelection(ids: number[]): void {
      const shown = ids.slice(0, 64);
      shown.forEach((id, i) => {
        const f = byId.get(id);
        if (!f) return;
        matrix.compose(
          new THREE.Vector3(f.position.x, f.position.y + 0.04, f.position.z),
          new THREE.Quaternion(),
          new THREE.Vector3(1, 1, 1),
        );
        rings.setMatrixAt(i, matrix);
      });
      rings.count = shown.length;
      rings.visible = shown.length > 0;
      rings.instanceMatrix.needsUpdate = true;
    },
  };
}
