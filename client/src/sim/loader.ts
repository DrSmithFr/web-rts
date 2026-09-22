// Raw surface registered by the Go WebAssembly module on globalThis.RTSSim.
export interface RawSim {
  version(): string;
  newWorld(seed?: string | number, players?: number): number;
  order(id: number, player: number, kind: number, payload?: Uint8Array): void;
  step(id: number): void;
  stepWith(id: number, buf: Uint8Array): void;
  brainNew(id: number, player: number, difficulty: string): number;
  debugSpawn(id: number, owner: number, defId: number, count: number): void;
  tick(id: number): number;
  hashHex(id: number): string;
  seed(id: number): number;
  mapSize(id: number): number;
  mapTiles(id: number): Uint8Array;
  mapHeights(id: number): Uint8Array;
  mapHashHex(id: number): string;
  spawnCount(id: number): number;
  spawnAt(id: number, index: number): number;
  unitsBlob(id: number): Uint8Array;
  nodesBlob(id: number): Uint8Array;
  buildingsBlob(id: number): Uint8Array;
  economyBlob(id: number): Uint8Array;
  defs(id: number): BuildingDefJson[];
  unitDefs(id: number): UnitDefJson[];
  eras(id: number): EraDefJson[];
  techDefs(id: number): TechDefJson[];
  playerTechs(id: number, player: number): number[];
  victory(id: number): { winner: number; kind: number };
  relicsBlob(id: number): Uint8Array;
  fogBlob(id: number, player: number): Uint8Array;
}

export interface EraDefJson {
  id: number;
  key: string;
  costFood: number;
  costGold: number;
  reqBuilding: number;
  ageTicks: number;
}

export interface TechDefJson {
  id: number;
  key: string;
  era: number;
  building: number;
  costFood: number;
  costWood: number;
  costGold: number;
  ticks: number;
  requires: number[];
}

export interface BuildingDefJson {
  id: number;
  key: string;
  costWood: number;
  costStone: number;
  buildTicks: number;
  popRoom: number;
  size: number;
  dropoff: boolean;
  trainFood: number;
  trainTicks: number;
  minEra: number;
}

export interface UnitBonusJson {
  class: number;
  bonus: number;
}

export interface UnitDefJson {
  id: number;
  key: string;
  class: number;
  hp: number;
  speed16: number;
  vision: number;
  damage: number;
  damageType: number;
  armorPierce: number;
  armorCrush: number;
  range16: number;
  rate: number;
  costFood: number;
  costWood: number;
  costGold: number;
  popCost: number;
  trainBy: number;
  trainTicks: number;
  corpseFood: number;
  flee: boolean;
  minEra: number;
  bonuses: UnitBonusJson[];
}

declare global {
  interface Window {
    RTSSim?: RawSim;
  }
}

const READY_ATTEMPTS = 200;
const READY_INTERVAL_MS = 10;

async function waitForSim(): Promise<RawSim> {
  for (let i = 0; i < READY_ATTEMPTS; i++) {
    if (window.RTSSim !== undefined) {
      return window.RTSSim;
    }
    await new Promise((resolve) => setTimeout(resolve, READY_INTERVAL_MS));
  }
  throw new Error('RTSSim bridge did not register');
}

// Instantiate sim.wasm through the Go runtime and resolve once the RTSSim
// globals exist. go.run resolves only when the module exits, which it never
// does, so its promise is intentionally ignored.
export async function loadSim(wasmUrl = '/sim.wasm'): Promise<RawSim> {
  const response = await fetch(wasmUrl);
  if (!response.ok) {
    throw new Error(`failed to fetch ${wasmUrl}: ${response.status}`);
  }
  const bytes = await response.arrayBuffer();
  const go = new Go();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance);
  return waitForSim();
}
