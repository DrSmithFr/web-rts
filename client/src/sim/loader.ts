// Raw surface registered by the Go WebAssembly module on globalThis.RTSSim.
export interface RawSim {
  version(): string;
  newWorld(seed?: number): number;
  order(id: number, player: number, kind: number, payload?: Uint8Array): void;
  step(id: number): void;
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
