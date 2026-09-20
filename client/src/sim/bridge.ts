import type { RawSim } from './loader';

// SimWorld is a typed handle on one simulation instance inside the wasm
// module. All gameplay numbers come from the simulation; the bridge only
// moves bytes.
export class SimWorld {
  private constructor(
    private readonly sim: RawSim,
    readonly id: number,
  ) {}

  static create(sim: RawSim, seed: number): SimWorld {
    return new SimWorld(sim, sim.newWorld(seed));
  }

  step(): void {
    this.sim.step(this.id);
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
