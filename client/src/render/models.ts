import * as THREE from 'three';
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js';

import { PALETTE } from './palette';

// Code-generated low-poly models (art direction: no imported meshes,
// under 300 triangles per unit). Each model splits into static geometry
// with baked vertex colors and animated rigid parts. Animated parts are
// authored in model space with their joint at the geometry origin so the
// renderer can pivot them with one matrix multiply per part.

export type PartRole = 'legF' | 'legB' | 'legL' | 'legR' | 'armR' | 'weapon' | 'carry';

export interface AnimPart {
  role: PartRole;
  geo: THREE.BufferGeometry;
  pivot: THREE.Vector3;
}

export interface UnitModel {
  body: THREE.BufferGeometry;
  insignia: THREE.BufferGeometry | null;
  anim: AnimPart[];
}

export interface BuildingModel {
  structure: THREE.BufferGeometry;
  insignia: THREE.BufferGeometry | null;
}

// Builder accumulates painted primitives.
export class Builder {
  readonly parts: THREE.BufferGeometry[] = [];

  box(w: number, h: number, d: number, x: number, y: number, z: number, hex: number, rx = 0, ry = 0): this {
    const g = new THREE.BoxGeometry(w, h, d);
    if (rx) g.rotateX(rx);
    if (ry) g.rotateY(ry);
    g.translate(x, y, z);
    this.parts.push(paint(g, hex));
    return this;
  }

  cyl(r1: number, r2: number, h: number, x: number, y: number, z: number, hex: number, seg = 6, rx = 0): this {
    const g = new THREE.CylinderGeometry(r1, r2, h, seg);
    if (rx) g.rotateX(rx);
    g.translate(x, y, z);
    this.parts.push(paint(g, hex));
    return this;
  }

  cone(r: number, h: number, x: number, y: number, z: number, hex: number, seg = 6, rx = 0, ry = 0): this {
    const g = new THREE.ConeGeometry(r, h, seg);
    if (rx) g.rotateX(rx);
    if (ry) g.rotateY(ry);
    g.translate(x, y, z);
    this.parts.push(paint(g, hex));
    return this;
  }

  merge(): THREE.BufferGeometry | null {
    if (this.parts.length === 0) return null;
    const m = mergeGeometries(this.parts, false);
    for (const p of this.parts) p.dispose();
    return m ?? null;
  }
}

export function paint(geo: THREE.BufferGeometry, hex: number): THREE.BufferGeometry {
  const c = new THREE.Color(hex);
  const n = geo.getAttribute('position').count;
  const colors = new Float32Array(n * 3);
  for (let i = 0; i < n; i++) {
    colors[i * 3] = c.r;
    colors[i * 3 + 1] = c.g;
    colors[i * 3 + 2] = c.b;
  }
  geo.setAttribute('color', new THREE.BufferAttribute(colors, 3));
  return geo;
}

const BONE = PALETTE.marble;

// Humanoid legs share one shape: cloth shin, leather boot, hip pivot.
function humanLegs(anim: AnimPart[], legHex: number): void {
  for (const role of ['legL', 'legR'] as const) {
    const b = new Builder();
    const x = role === 'legL' ? -0.075 : 0.075;
    b.box(0.1, 0.28, 0.12, 0, -0.14, 0, legHex);
    b.box(0.11, 0.05, 0.16, 0, -0.285, 0.02, PALETTE.leather);
    anim.push({ role, geo: b.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(x, 0.29, 0) });
  }
}

// Quadruped legs: one geometry holds both legs of one end of the body.
function quadLegs(anim: AnimPart[], hex: number, pivotZ: number, front: boolean, len: number, w: number, spread: number): void {
  const b = new Builder();
  b.box(w, len, w * 1.1, -spread, -len / 2, 0, hex);
  b.box(w, len, w * 1.1, spread, -len / 2, 0, hex);
  anim.push({
    role: front ? 'legF' : 'legB',
    geo: b.merge() as THREE.BufferGeometry,
    pivot: new THREE.Vector3(0, len + 0.01, pivotZ),
  });
}

function villager(): UnitModel {
  const body = new Builder();
  body.box(0.21, 0.21, 0.19, 0, 0.7, 0, PALETTE.skin);
  body.box(0.08, 0.22, 0.1, -0.2, 0.5, 0, PALETTE.skin);
  const insignia = new Builder();
  insignia.box(0.3, 0.34, 0.21, 0, 0.44, 0, 0xffffff);
  const anim: AnimPart[] = [];
  humanLegs(anim, PALETTE.cloth);
  const arm = new Builder();
  arm.box(0.08, 0.22, 0.1, 0, -0.11, 0, PALETTE.skin);
  anim.push({ role: 'armR', geo: arm.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.19, 0.58, 0) });
  const tool = new Builder();
  tool.cyl(0.02, 0.02, 0.36, 0, 0.06, 0.05, PALETTE.trunk, 5, 0.5);
  tool.box(0.09, 0.05, 0.13, 0, 0.24, 0.15, PALETTE.iron);
  anim.push({ role: 'weapon', geo: tool.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.21, 0.5, 0.04) });
  const carry = new Builder();
  carry.box(0.15, 0.17, 0.15, 0.02, 0.14, 0, 0xffffff);
  anim.push({ role: 'carry', geo: carry.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.14, 0.6, -0.02) });
  return { body: body.merge() as THREE.BufferGeometry, insignia: insignia.merge(), anim };
}

function lancer(): UnitModel {
  const body = new Builder();
  body.box(0.21, 0.21, 0.19, 0, 0.72, 0, PALETTE.skin);
  body.box(0.23, 0.09, 0.21, 0, 0.84, 0, PALETTE.steel);
  body.box(0.07, 0.3, 0.13, -0.23, 0.5, 0.04, PALETTE.leather);
  const insignia = new Builder();
  insignia.box(0.32, 0.36, 0.22, 0, 0.46, 0, 0xffffff);
  insignia.box(0.09, 0.09, 0.21, -0.21, 0.65, 0, 0xffffff);
  insignia.box(0.09, 0.09, 0.21, 0.21, 0.65, 0, 0xffffff);
  const anim: AnimPart[] = [];
  humanLegs(anim, PALETTE.iron);
  const spear = new Builder();
  spear.cyl(0.022, 0.022, 1.05, 0, 0.22, 0, PALETTE.trunk, 5);
  spear.cone(0.05, 0.24, 0, 0.87, 0, PALETTE.steel, 5);
  anim.push({ role: 'weapon', geo: spear.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.23, 0.52, 0.1) });
  return { body: body.merge() as THREE.BufferGeometry, insignia: insignia.merge(), anim };
}

function archer(): UnitModel {
  const body = new Builder();
  body.box(0.21, 0.21, 0.19, 0, 0.7, 0, PALETTE.skin);
  body.box(0.23, 0.11, 0.21, 0, 0.81, 0, PALETTE.cloth);
  body.box(0.07, 0.2, 0.08, -0.19, 0.52, 0.13, PALETTE.skin);
  const insignia = new Builder();
  insignia.box(0.3, 0.34, 0.21, 0, 0.45, 0, 0xffffff);
  const anim: AnimPart[] = [];
  humanLegs(anim, PALETTE.cloth);
  const arm = new Builder();
  arm.box(0.07, 0.2, 0.08, 0, -0.1, 0, PALETTE.skin);
  anim.push({ role: 'armR', geo: arm.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.16, 0.56, 0.02) });
  const bow = new Builder();
  const arc = new THREE.TorusGeometry(0.24, 0.018, 4, 10, Math.PI * 1.25);
  arc.rotateY(Math.PI / 2);
  arc.rotateZ(Math.PI * 0.87);
  bow.parts.push(paint(arc, PALETTE.trunk));
  anim.push({ role: 'weapon', geo: bow.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(-0.02, 0.52, 0.2) });
  return { body: body.merge() as THREE.BufferGeometry, insignia: insignia.merge(), anim };
}

function scout(): UnitModel {
  const body = new Builder();
  body.box(0.36, 0.36, 0.8, 0, 0.52, 0, PALETTE.mount);
  body.box(0.16, 0.3, 0.18, 0, 0.74, 0.3, PALETTE.mount, -0.5);
  body.box(0.14, 0.15, 0.3, 0, 0.88, 0.52, PALETTE.mount, -0.15);
  body.cone(0.08, 0.36, 0, 0.62, -0.52, PALETTE.mane, 5, 2.3);
  body.box(0.19, 0.19, 0.18, 0, 1.04, 0.02, PALETTE.skin);
  body.box(0.12, 0.24, 0.32, -0.21, 0.72, 0, PALETTE.leather);
  body.box(0.12, 0.24, 0.32, 0.21, 0.72, 0, PALETTE.leather);
  const insignia = new Builder();
  insignia.box(0.24, 0.3, 0.2, 0, 0.87, 0.02, 0xffffff);
  const anim: AnimPart[] = [];
  quadLegs(anim, PALETTE.mount, 0.24, true, 0.5, 0.11, 0.09);
  quadLegs(anim, PALETTE.mount, -0.24, false, 0.52, 0.11, 0.09);
  const lance = new Builder();
  lance.cyl(0.02, 0.02, 1.2, 0, 0.34, 0, PALETTE.trunk, 5);
  lance.cone(0.05, 0.22, 0, 1.05, 0, PALETTE.steel, 5);
  anim.push({ role: 'weapon', geo: lance.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.18, 0.74, 0.04) });
  return { body: body.merge() as THREE.BufferGeometry, insignia: insignia.merge(), anim };
}

function knight(): UnitModel {
  const body = new Builder();
  body.box(0.24, 0.24, 0.22, 0, 0.73, 0, PALETTE.steel);
  body.box(0.1, 0.13, 0.21, -0.22, 0.66, 0, PALETTE.steel);
  body.box(0.1, 0.13, 0.21, 0.22, 0.66, 0, PALETTE.steel);
  body.box(0.07, 0.34, 0.2, -0.25, 0.5, 0.02, PALETTE.iron);
  const insignia = new Builder();
  insignia.box(0.34, 0.44, 0.24, 0, 0.44, 0, 0xffffff);
  const anim: AnimPart[] = [];
  humanLegs(anim, PALETTE.iron);
  const sword = new Builder();
  sword.box(0.04, 0.6, 0.1, 0, 0.36, 0, PALETTE.steel);
  sword.box(0.16, 0.04, 0.05, 0, 0.05, 0, PALETTE.iron);
  sword.cyl(0.02, 0.02, 0.14, 0, -0.03, 0, PALETTE.leather, 5);
  anim.push({ role: 'weapon', geo: sword.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.25, 0.54, 0.06) });
  return { body: body.merge() as THREE.BufferGeometry, insignia: insignia.merge(), anim };
}

function crossbowman(): UnitModel {
  const body = new Builder();
  body.box(0.21, 0.21, 0.19, 0, 0.7, 0, PALETTE.skin);
  body.box(0.23, 0.1, 0.21, 0, 0.81, 0, PALETTE.leather);
  body.box(0.07, 0.2, 0.14, -0.17, 0.52, 0.14, PALETTE.skin);
  const insignia = new Builder();
  insignia.box(0.3, 0.34, 0.21, 0, 0.45, 0, 0xffffff);
  const anim: AnimPart[] = [];
  humanLegs(anim, PALETTE.cloth);
  const bow = new Builder();
  bow.box(0.05, 0.06, 0.42, 0, 0.04, 0.1, PALETTE.trunk);
  bow.box(0.36, 0.04, 0.05, 0, 0.08, 0.26, PALETTE.iron);
  anim.push({ role: 'weapon', geo: bow.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0.16, 0.52, 0.02) });
  return { body: body.merge() as THREE.BufferGeometry, insignia: insignia.merge(), anim };
}

function deer(): UnitModel {
  const body = new Builder();
  body.box(0.28, 0.3, 0.6, 0, 0.42, 0, PALETTE.deer);
  body.box(0.12, 0.22, 0.14, 0, 0.6, 0.28, PALETTE.deer, -0.6);
  body.box(0.12, 0.13, 0.22, 0, 0.66, 0.42, PALETTE.deer, -0.15);
  body.box(0.04, 0.09, 0.03, -0.06, 0.75, 0.36, PALETTE.mane);
  body.box(0.04, 0.09, 0.03, 0.06, 0.75, 0.36, PALETTE.mane);
  body.cone(0.02, 0.2, -0.05, 0.84, 0.38, BONE, 4, -0.25);
  body.cone(0.02, 0.2, 0.05, 0.84, 0.38, BONE, 4, 0.25);
  const anim: AnimPart[] = [];
  quadLegs(anim, PALETTE.deer, 0.2, true, 0.36, 0.07, 0.09);
  quadLegs(anim, PALETTE.deer, -0.2, false, 0.38, 0.07, 0.09);
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function boar(): UnitModel {
  const body = new Builder();
  body.box(0.34, 0.3, 0.62, 0, 0.34, 0, PALETTE.boar);
  body.box(0.13, 0.11, 0.14, 0, 0.28, 0.38, PALETTE.boar);
  body.cone(0.025, 0.1, -0.06, 0.33, 0.44, BONE, 4, -2.1);
  body.cone(0.025, 0.1, 0.06, 0.33, 0.44, BONE, 4, -2.1);
  body.box(0.09, 0.09, 0.03, -0.11, 0.47, 0.26, PALETTE.mane);
  body.box(0.09, 0.09, 0.03, 0.11, 0.47, 0.26, PALETTE.mane);
  const anim: AnimPart[] = [];
  quadLegs(anim, PALETTE.mane, 0.2, true, 0.24, 0.08, 0.1);
  quadLegs(anim, PALETTE.mane, -0.2, false, 0.24, 0.08, 0.1);
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function rabbit(): UnitModel {
  const body = new Builder();
  body.box(0.2, 0.17, 0.26, 0, 0.16, -0.02, PALETTE.rabbit);
  body.box(0.12, 0.12, 0.12, 0, 0.24, 0.17, PALETTE.rabbit);
  body.box(0.035, 0.12, 0.04, -0.04, 0.36, 0.16, PALETTE.rabbit);
  body.box(0.035, 0.12, 0.04, 0.04, 0.36, 0.16, PALETTE.rabbit);
  body.box(0.05, 0.05, 0.05, 0, 0.16, -0.18, 0xf4efe6);
  const anim: AnimPart[] = [];
  quadLegs(anim, PALETTE.rabbit, 0.09, true, 0.12, 0.05, 0.05);
  quadLegs(anim, PALETTE.rabbit, -0.09, false, 0.14, 0.06, 0.06);
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function sheep(): UnitModel {
  const body = new Builder();
  body.box(0.3, 0.26, 0.44, 0, 0.34, 0, PALETTE.sheep);
  body.box(0.13, 0.13, 0.12, 0, 0.32, 0.3, PALETTE.sheepFace);
  body.box(0.05, 0.05, 0.03, -0.085, 0.37, 0.29, PALETTE.sheepFace);
  body.box(0.05, 0.05, 0.03, 0.085, 0.37, 0.29, PALETTE.sheepFace);
  const anim: AnimPart[] = [];
  quadLegs(anim, PALETTE.sheepFace, 0.15, true, 0.22, 0.07, 0.08);
  quadLegs(anim, PALETTE.sheepFace, -0.15, false, 0.22, 0.07, 0.08);
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function bear(): UnitModel {
  const body = new Builder();
  body.box(0.42, 0.4, 0.72, 0, 0.44, 0, PALETTE.bear);
  body.box(0.24, 0.18, 0.16, 0, 0.4, 0.44, PALETTE.bear);
  body.box(0.12, 0.08, 0.1, 0, 0.42, 0.52, 0x7a5c40);
  body.box(0.07, 0.07, 0.04, -0.12, 0.54, 0.4, PALETTE.bear);
  body.box(0.07, 0.07, 0.04, 0.12, 0.54, 0.4, PALETTE.bear);
  const anim: AnimPart[] = [];
  quadLegs(anim, PALETTE.bear, 0.24, true, 0.34, 0.12, 0.12);
  quadLegs(anim, PALETTE.bear, -0.24, false, 0.34, 0.12, 0.12);
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function wolfOf(hex: number, maneHex: number): UnitModel {
  const body = new Builder();
  body.box(0.24, 0.24, 0.54, 0, 0.36, 0, hex);
  body.box(0.14, 0.13, 0.18, 0, 0.42, 0.34, hex);
  body.box(0.08, 0.07, 0.1, 0, 0.38, 0.48, maneHex);
  body.box(0.05, 0.06, 0.03, -0.07, 0.51, 0.31, hex);
  body.box(0.05, 0.06, 0.03, 0.07, 0.51, 0.31, hex);
  body.box(0.06, 0.06, 0.22, 0, 0.4, -0.34, maneHex);
  const anim: AnimPart[] = [];
  quadLegs(anim, hex, 0.19, true, 0.3, 0.07, 0.08);
  quadLegs(anim, hex, -0.19, false, 0.32, 0.07, 0.08);
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function zombie(): UnitModel {
  const body = new Builder();
  body.box(0.24, 0.5, 0.28, 0, 0.5, 0, 0x6e8a5a);
  body.box(0.18, 0.18, 0.18, 0, 0.82, 0.04, 0x86a06e);
  body.box(0.07, 0.3, 0.07, -0.19, 0.6, 0.08, 0x6e8a5a, 0.5);
  body.box(0.07, 0.3, 0.07, 0.19, 0.6, 0.08, 0x6e8a5a, 0.5);
  const anim: AnimPart[] = [];
  quadLegs(anim, 0x55693f, 0.08, true, 0.26, 0.07, 0.09);
  quadLegs(anim, 0x55693f, -0.08, false, 0.26, 0.07, 0.09);
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function ram(): UnitModel {
  const body = new Builder();
  body.box(0.7, 0.3, 0.34, 0, 0.24, 0, PALETTE.plank);
  body.box(0.5, 0.22, 0.3, 0, 0.44, 0, PALETTE.trunk);
  body.box(0.5, 0.06, 0.06, 0, 0.6, 0, PALETTE.trunk);
  const anim: AnimPart[] = [];
  for (const [role, z] of [['legF', 0.22], ['legB', -0.22]] as const) {
    const wheel = new Builder();
    for (const x of [-0.24, 0.24]) {
      wheel.cyl(0.13, 0.13, 0.05, x, 0, 0, PALETTE.trunk, 6);
    }
    anim.push({ role, geo: wheel.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0, 0.13, z) });
  }
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

function trebuchet(): UnitModel {
  const body = new Builder();
  body.box(0.56, 0.08, 0.2, 0, 0.1, 0, PALETTE.trunk, 0.4);
  body.box(0.56, 0.08, 0.2, 0, 0.1, 0, PALETTE.trunk, -0.4);
  body.box(0.1, 0.34, 0.1, 0, 0.3, 0, PALETTE.plank);
  const anim: AnimPart[] = [];
  const arm = new Builder();
  arm.box(0.06, 0.6, 0.06, 0, 0.1, 0, PALETTE.trunk);
  arm.box(0.14, 0.14, 0.14, -0.24, -0.1, 0, PALETTE.stone);
  anim.push({ role: 'armR', geo: arm.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0, 0.48, 0) });
  const sling = new Builder();
  sling.box(0.12, 0.06, 0.12, 0, -0.32, 0, PALETTE.leather);
  anim.push({ role: 'weapon', geo: sling.merge() as THREE.BufferGeometry, pivot: new THREE.Vector3(0, 0.86, 0) });
  return { body: body.merge() as THREE.BufferGeometry, insignia: null, anim };
}

const UNIT_MODELS: Record<number, () => UnitModel> = {
  1: villager,
  2: lancer,
  3: archer,
  4: scout,
  5: deer,
  6: boar,
  7: knight,
  8: crossbowman,
  9: rabbit,
  10: sheep,
  11: bear,
  12: () => wolfOf(PALETTE.wolf, 0xb9bdc4),
  13: () => wolfOf(PALETTE.blackWolf, 0x1f2126),
  14: zombie,
  15: ram,
  16: trebuchet,
};

const modelCache = new Map<number, UnitModel>();

export function unitModel(def: number): UnitModel {
  const hit = modelCache.get(def);
  if (hit) return hit;
  const make = UNIT_MODELS[def] ?? villager;
  const m = make();
  modelCache.set(def, m);
  return m;
}

export const UNIT_DEFS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16];

// --- buildings ---------------------------------------------------------

function hut(): BuildingModel {
  const s = new Builder();
  s.box(1.7, 0.12, 1.7, 0, 0.06, 0, PALETTE.plank);
  s.box(1.5, 0.62, 1.5, 0, 0.37, 0, PALETTE.plaster);
  s.cone(1.32, 0.55, 0, 0.96, 0, PALETTE.thatch, 4, 0, Math.PI / 4);
  s.box(0.3, 0.42, 0.06, 0, 0.27, 0.76, PALETTE.trunk);
  const ins = new Builder();
  ins.cyl(0.02, 0.02, 0.7, 0.72, 0.85, 0.72, PALETTE.trunk, 4);
  ins.box(0.26, 0.15, 0.03, 0.86, 1.06, 0.72, 0xffffff);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function campfire(): BuildingModel {
  const s = new Builder();
  for (let i = 0; i < 7; i++) {
    const a = (i / 7) * Math.PI * 2;
    s.box(0.13, 0.09, 0.1, Math.cos(a) * 0.32, 0.04, Math.sin(a) * 0.32, PALETTE.stone, 0, a);
  }
  s.box(0.5, 0.06, 0.1, 0, 0.05, 0, PALETTE.trunk, 0, 0.6);
  s.box(0.5, 0.06, 0.1, 0, 0.1, 0, PALETTE.trunk, 0, -0.6);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: null };
}

function storehouse(): BuildingModel {
  const s = new Builder();
  s.box(1.7, 0.22, 1.7, 0, 0.11, 0, PALETTE.plank);
  s.box(1.4, 0.5, 1.4, 0, 0.47, 0, PALETTE.plank);
  s.cone(1.22, 0.4, 0, 0.86, 0, PALETTE.thatch, 4, 0, Math.PI / 4);
  s.box(0.5, 0.07, 0.6, 0, 0.13, 1.0, PALETTE.plank, -0.4);
  const ins = new Builder();
  ins.box(0.32, 0.22, 0.03, 0, 0.56, 0.72, 0xffffff);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function barracks(): BuildingModel {
  const s = new Builder();
  s.box(1.7, 0.6, 1.5, 0, 0.3, 0, PALETTE.plaster);
  s.cone(1.42, 0.45, 0, 0.84, 0, PALETTE.thatch, 4, 0, Math.PI / 4);
  for (const [x, z] of [[-0.8, -0.7], [0.8, -0.7], [-0.8, 0.7], [0.8, 0.7]]) {
    s.cyl(0.05, 0.06, 0.66, x, 0.33, z, PALETTE.trunk, 5);
  }
  const ins = new Builder();
  ins.cyl(0.025, 0.025, 1.1, 0.92, 0.7, 0.62, PALETTE.trunk, 4);
  ins.box(0.34, 0.42, 0.03, 1.1, 0.86, 0.62, 0xffffff);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function market(): BuildingModel {
  const s = new Builder();
  s.box(1.5, 0.36, 0.5, 0, 0.18, 0.2, PALETTE.plank);
  for (const [x, z] of [[-0.75, -0.45], [0.75, -0.45], [-0.75, 0.55], [0.75, 0.55]]) {
    s.cyl(0.03, 0.03, 0.85, x, 0.42, z, PALETTE.trunk, 5);
  }
  s.box(0.26, 0.26, 0.26, -0.62, 0.13, -0.4, PALETTE.trunk);
  s.box(0.2, 0.2, 0.2, 0.66, 0.1, -0.42, PALETTE.trunk);
  const ins = new Builder();
  ins.box(1.85, 0.06, 1.0, 0, 0.86, 0.05, 0xffffff, 0.32);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function forge(): BuildingModel {
  const s = new Builder();
  s.box(1.6, 0.5, 1.5, 0, 0.25, 0, PALETTE.stone);
  s.box(0.3, 0.9, 0.3, 0.5, 0.7, -0.3, PALETTE.stone);
  s.cone(1.25, 0.35, 0, 0.71, 0, PALETTE.thatch, 4, 0, Math.PI / 4);
  s.box(0.3, 0.22, 0.06, 0, 0.28, 0.76, PALETTE.ember);
  const ins = new Builder();
  ins.box(0.34, 0.16, 0.03, 0, 0.66, -0.77, 0xffffff);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function bank(): BuildingModel {
  const s = new Builder();
  s.box(1.7, 0.16, 1.7, 0, 0.08, 0, PALETTE.marble);
  for (const [x, z] of [[-0.55, -0.55], [0.55, -0.55], [-0.55, 0.55], [0.55, 0.55]]) {
    s.cyl(0.09, 0.1, 0.72, x, 0.52, z, PALETTE.marble, 6);
  }
  s.box(1.6, 0.14, 1.6, 0, 0.95, 0, PALETTE.marble);
  s.cone(1.2, 0.4, 0, 1.22, 0, PALETTE.marble, 4, 0, Math.PI / 4);
  const ins = new Builder();
  ins.box(0.36, 0.5, 0.05, 0, 0.44, 0.82, PALETTE.leather);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function wonder(): BuildingModel {
  const s = new Builder();
  s.box(2.7, 0.3, 2.7, 0, 0.15, 0, PALETTE.marble);
  s.box(2.1, 0.3, 2.1, 0, 0.45, 0, PALETTE.marble);
  s.box(1.5, 0.32, 1.5, 0, 0.76, 0, PALETTE.marble);
  s.box(0.9, 0.4, 0.9, 0, 1.12, 0, PALETTE.plaster);
  s.cone(0.5, 1.0, 0, 1.82, 0, PALETTE.gold, 4, 0, Math.PI / 4);
  const ins = new Builder();
  for (const [x, z] of [[-1.25, -1.25], [1.25, -1.25], [-1.25, 1.25], [1.25, 1.25]]) {
    ins.cyl(0.02, 0.02, 0.6, x, 0.6, z, PALETTE.trunk, 4);
    ins.box(0.2, 0.13, 0.02, x + (x > 0 ? 0.12 : -0.12), 0.82, z, 0xffffff);
  }
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function wall(): BuildingModel {
  const s = new Builder();
  s.box(1.05, 0.12, 1.05, 0, 0.06, 0, PALETTE.stone);
  s.box(0.8, 0.72, 0.8, 0, 0.42, 0, PALETTE.stone);
  for (const [x, z] of [[-0.4, -0.4], [0.4, -0.4], [-0.4, 0.4], [0.4, 0.4]]) {
    s.box(0.22, 0.2, 0.22, x, 0.88, z, PALETTE.stone);
  }
  return { structure: s.merge() as THREE.BufferGeometry, insignia: null };
}

function watchtower(): BuildingModel {
  const s = new Builder();
  s.box(0.9, 0.14, 0.9, 0, 0.07, 0, PALETTE.stone);
  s.box(0.62, 1.1, 0.62, 0, 0.62, 0, PALETTE.stone);
  s.box(0.86, 0.12, 0.86, 0, 1.22, 0, PALETTE.plank);
  for (const [x, z] of [[-0.4, -0.4], [0.4, -0.4], [-0.4, 0.4], [0.4, 0.4]]) {
    s.box(0.16, 0.24, 0.16, x, 1.4, z, PALETTE.stone);
  }
  const ins = new Builder();
  ins.box(0.26, 0.16, 0.03, 0, 0.9, 0.33, 0xffffff);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: ins.merge() };
}

function siegeWorkshop(): BuildingModel {
  const s = new Builder();
  s.box(1.7, 0.14, 1.7, 0, 0.07, 0, PALETTE.plank);
  s.box(1.3, 0.55, 1.3, 0, 0.4, 0, PALETTE.trunk);
  s.cone(1.22, 0.4, 0, 0.86, 0, PALETTE.thatch, 4, 0, Math.PI / 4);
  s.box(0.14, 0.7, 0.14, 0.7, 0.35, 0.7, PALETTE.trunk, 0.6);
  s.box(0.2, 0.2, 0.2, -0.6, 0.42, -0.6, PALETTE.stone);
  return { structure: s.merge() as THREE.BufferGeometry, insignia: null };
}

const BUILDING_MODELS: Record<string, () => BuildingModel> = {
  hut,
  campfire,
  storehouse,
  barracks,
  market,
  forge,
  bank,
  wonder,
  wall,
  watchtower,
  siege_workshop: siegeWorkshop,
};

const buildingCache = new Map<string, BuildingModel>();

export function buildingModel(key: string): BuildingModel {
  const hit = buildingCache.get(key);
  if (hit) return hit;
  const make = BUILDING_MODELS[key] ?? hut;
  const m = make();
  buildingCache.set(key, m);
  return m;
}
