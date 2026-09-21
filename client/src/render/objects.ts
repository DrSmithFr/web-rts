import * as THREE from 'three';

import { HEIGHT_SCALE, PALETTE } from './palette';

// Static map decor: spawn platform discs. Resource nodes and units live in
// render/nodes.ts and render/actors.ts and follow simulation state.

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

export function buildMapObjects(
  size: number,
  heights: Uint16Array,
  spawns: { x: number; y: number }[],
): MapObjects {
  const group = new THREE.Group();
  const features: MapFeature[] = [];

  const groundY = (tx: number, ty: number): number =>
    heights[ty * (size + 1) + tx] * HEIGHT_SCALE;

  const markerGeo = new THREE.CylinderGeometry(1.1, 1.1, 0.12, 8);
  const markerMat = new THREE.MeshLambertMaterial({ color: PALETTE.marker, flatShading: true });
  const markers = new THREE.InstancedMesh(markerGeo, markerMat, Math.max(spawns.length, 1));

  const matrix = new THREE.Matrix4();

  spawns.forEach((s, i) => {
    const y = groundY(s.x, s.y) + 0.06;
    matrix.compose(new THREE.Vector3(s.x + 0.5, y, s.y + 0.5), new THREE.Quaternion(), new THREE.Vector3(1, 1, 1));
    markers.setMatrixAt(i, matrix);
    features.push({ id: s.y * size + s.x, position: new THREE.Vector3(s.x + 0.5, y, s.y + 0.5) });
  });

  markers.instanceMatrix.needsUpdate = true;
  group.add(markers);

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
