// FogView decodes the 2-bit-per-tile fog blob from the sim bridge and
// answers per-tile queries for the renderer.

export const FogUnexplored = 0;
export const FogExplored = 1;
export const FogVisible = 2;

export class FogView {
  private states: Uint8Array;

  constructor(
    readonly size: number,
    packed: Uint8Array,
  ) {
    this.states = new Uint8Array(size * size);
    this.update(packed);
  }

  update(packed: Uint8Array): void {
    const n = this.size * this.size;
    for (let i = 0; i < n; i++) {
      this.states[i] = (packed[i >> 2] >> (2 * (i & 3))) & 3;
    }
  }

  // at returns the fog state of a tile; out of bounds is unexplored.
  at(x: number, y: number): number {
    if (x < 0 || y < 0 || x >= this.size || y >= this.size) {
      return FogUnexplored;
    }
    return this.states[y * this.size + x];
  }

  visible(x: number, y: number): boolean {
    return this.at(x, y) !== FogUnexplored;
  }
}

// groundTint returns a brightness multiplier for a fog state.
export function groundTint(state: number): number {
  switch (state) {
    case FogVisible:
      return 1;
    case FogExplored:
      return 0.4;
    default:
      return 0.08;
  }
}
