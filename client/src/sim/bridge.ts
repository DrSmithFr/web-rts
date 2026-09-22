import type {
  BuildingDefJson,
  EraDefJson,
  RawSim,
  TechDefJson,
  UnitDefJson,
} from './loader';

export interface UnitRec {
  id: number;
  owner: number;
  kind: number;
  def: number;
  state: number;
  carriedKind: number;
  carried: number;
  x: number;
  y: number;
  hp: number;
  target: number;
}

export interface NodeRec {
  id: number;
  kind: number;
  amount: number;
  x: number;
  y: number;
  tileX: number;
  tileY: number;
}

export interface BuildingRec {
  id: number;
  def: number;
  owner: number;
  progress: number;
  trainCur: number;
  trainLeft: number;
  trainReq: number;
  trainQueue: number[];
  tileX: number;
  tileY: number;
}

export interface PlayerEcon {
  active: boolean;
  pop: number;
  popCap: number;
  res: number[];
  era: number;
  ageTarget: number;
  ageLeft: number;
  techCur: number;
  techLeft: number;
}

export interface Victory {
  winner: number;
  kind: number;
}

export interface RelicRec {
  x: number;
  y: number;
  owner: number;
}

export type BuildingDef = BuildingDefJson;
export type UnitDef = UnitDefJson;
export type EraDef = EraDefJson;
export type TechDef = TechDefJson;

// SimWorld is a typed handle on one simulation instance inside the wasm
// module. All gameplay numbers come from the simulation; the bridge only
// moves bytes.
export class SimWorld {
  private constructor(
    private readonly sim: RawSim,
    readonly id: number,
  ) {}

  static create(sim: RawSim, seed: string | number, players?: number): SimWorld {
    return new SimWorld(sim, players ? sim.newWorld(seed, players) : sim.newWorld(seed));
  }

  step(): void {
    this.sim.step(this.id);
  }

  brainNew(player: number, difficulty: string): number {
    return this.sim.brainNew(this.id, player, difficulty);
  }

  debugSpawn(owner: number, defId: number, count: number): void {
    this.sim.debugSpawn(this.id, owner, defId, count);
  }

  stepWith(orders: { player: number; kind: number; payload: Uint8Array }[]): void {
    let len = 2;
    for (const o of orders) len += 5 + o.payload.length;
    const buf = new Uint8Array(len);
    const dv = new DataView(buf.buffer);
    dv.setUint16(0, orders.length, true);
    let off = 2;
    for (const o of orders) {
      dv.setUint8(off, o.player);
      dv.setUint16(off + 1, o.kind, true);
      dv.setUint16(off + 3, o.payload.length, true);
      buf.set(o.payload, off + 5);
      off += 5 + o.payload.length;
    }
    this.sim.stepWith(this.id, buf);
  }

  order(player: number, kind: number, payload?: Uint8Array): void {
    this.sim.order(this.id, player, kind, payload);
  }

  tick(): number {
    return this.sim.tick(this.id);
  }

  hashHex(): string {
    return this.sim.hashHex(this.id);
  }

  seed(): number {
    return this.sim.seed(this.id);
  }

  mapSize(): number {
    return this.sim.mapSize(this.id);
  }

  mapTiles(): Uint8Array {
    return this.sim.mapTiles(this.id);
  }

  // vertexHeights decodes the (size+1)^2 whole-number vertex heights.
  vertexHeights(): Uint16Array {
    const raw = this.sim.mapHeights(this.id);
    const view = new DataView(raw.buffer, raw.byteOffset, raw.byteLength);
    const out = new Uint16Array(raw.byteLength / 2);
    for (let i = 0; i < out.length; i++) {
      out[i] = view.getUint16(i * 2, true);
    }
    return out;
  }

  mapHashHex(): string {
    return this.sim.mapHashHex(this.id);
  }

  spawns(): { x: number; y: number }[] {
    const n = this.sim.spawnCount(this.id);
    const out: { x: number; y: number }[] = [];
    for (let i = 0; i < n; i++) {
      const packed = this.sim.spawnAt(this.id, i);
      out.push({ x: Math.floor(packed / 65536), y: packed % 65536 });
    }
    return out;
  }

  // --- phase 3: dynamic state ---------------------------------------------

  units(): UnitRec[] {
    const raw = this.sim.unitsBlob(this.id);
    const v = new DataView(raw.buffer, raw.byteOffset, raw.byteLength);
    const out: UnitRec[] = [];
    for (let o = 0; o + 24 <= raw.byteLength; o += 24) {
      out.push({
        id: v.getUint32(o, true),
        owner: raw[o + 4],
        kind: raw[o + 5],
        def: v.getUint16(o + 14, true),
        state: raw[o + 6],
        carriedKind: raw[o + 7],
        x: v.getUint16(o + 8, true) / 16,
        y: v.getUint16(o + 10, true) / 16,
        hp: v.getUint16(o + 12, true),
        target: v.getUint32(o + 16, true),
        carried: v.getUint16(o + 20, true),
      });
    }
    return out;
  }

  nodes(): NodeRec[] {
    const raw = this.sim.nodesBlob(this.id);
    const v = new DataView(raw.buffer, raw.byteOffset, raw.byteLength);
    const out: NodeRec[] = [];
    for (let o = 0; o + 16 <= raw.byteLength; o += 16) {
      out.push({
        id: v.getUint32(o, true),
        kind: raw[o + 4],
        amount: v.getUint16(o + 5, true),
        x: v.getUint16(o + 7, true) / 16,
        y: v.getUint16(o + 9, true) / 16,
        tileX: v.getUint16(o + 11, true),
        tileY: v.getUint16(o + 13, true),
      });
    }
    return out;
  }

  buildings(): BuildingRec[] {
    const raw = this.sim.buildingsBlob(this.id);
    const v = new DataView(raw.buffer, raw.byteOffset, raw.byteLength);
    const out: BuildingRec[] = [];
    for (let o = 0; o + 30 <= raw.byteLength; o += 30) {
      out.push({
        id: v.getUint32(o, true),
        def: v.getUint16(o + 4, true),
        owner: raw[o + 6],
        progress: v.getUint16(o + 8, true),
        trainCur: v.getUint16(o + 10, true),
        trainLeft: v.getUint16(o + 12, true),
        trainReq: v.getUint16(o + 14, true),
        tileX: v.getUint16(o + 16, true),
        tileY: v.getUint16(o + 18, true),
        trainQueue: [0, 1, 2, 3, 4].map((k) => v.getUint16(o + 20 + k * 2, true)),
      });
    }
    return out;
  }

  economy(): PlayerEcon[] {
    const raw = this.sim.economyBlob(this.id);
    const v = new DataView(raw.buffer, raw.byteOffset, raw.byteLength);
    const out: PlayerEcon[] = [];
    for (let o = 0; o + 40 <= raw.byteLength; o += 40) {
      out.push({
        active: raw[o] !== 0,
        pop: v.getUint16(o + 1, true),
        popCap: v.getUint16(o + 3, true),
        res: [0, 1, 2, 3, 4, 5].map((k) => v.getUint32(o + 5 + k * 4, true)),
        era: raw[o + 32],
        ageTarget: raw[o + 33],
        ageLeft: v.getUint16(o + 34, true),
        techCur: v.getUint16(o + 36, true),
        techLeft: v.getUint16(o + 38, true),
      });
    }
    return out;
  }

  eras(): EraDef[] {
    return this.sim.eras(this.id);
  }

  techDefs(): TechDef[] {
    return this.sim.techDefs(this.id);
  }

  playerTechs(player: number): number[] {
    return this.sim.playerTechs(this.id, player);
  }

  victory(): Victory {
    return this.sim.victory(this.id);
  }

  relics(): RelicRec[] {
    const raw = this.sim.relicsBlob(this.id);
    const v = new DataView(raw.buffer, raw.byteOffset, raw.byteLength);
    const out: RelicRec[] = [];
    for (let o = 0; o + 5 <= raw.byteLength; o += 5) {
      out.push({ x: v.getUint16(o, true), y: v.getUint16(o + 2, true), owner: raw[o + 4] });
    }
    return out;
  }

  defs(): BuildingDef[] {
    return this.sim.defs(this.id);
  }

  unitDefs(): UnitDef[] {
    return this.sim.unitDefs(this.id);
  }

  fogBlob(player: number): Uint8Array {
    return this.sim.fogBlob(this.id, player);
  }
}

// Seed comes from the ?seed= query parameter, as a decimal or 0x integer
// string, falling back to the fixed phase-1 seed.
export function seedFromLocation(search: string): number {
  const raw = new URLSearchParams(search).get('seed');
  if (raw === null) {
    return 0x5eed;
  }
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed >= 0 ? parsed : 0x5eed;
}
