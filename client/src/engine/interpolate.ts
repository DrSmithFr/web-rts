// Interpolation between 20 Hz simulation ticks for 60 fps rendering.
// Positions are kept per entity id; each frame callers pass a blend
// factor in [0,1] between the previous and current tick snapshots.

export interface Vec2 {
  x: number;
  y: number;
}

export class Interpolated {
  private prev = new Map<number, Vec2>();
  private curr = new Map<number, Vec2>();

  // commit replaces the snapshots for one tick.
  commit(entries: Iterable<[number, Vec2]>): void {
    this.prev = this.curr;
    this.curr = new Map();
    for (const [id, p] of entries) this.curr.set(id, p);
  }

  // get blends previous into current; entities seen for the first time
  // render at their current position (no interpolation from nowhere).
  get(id: number, alpha: number): Vec2 | undefined {
    const c = this.curr.get(id);
    if (!c) return undefined;
    const p = this.prev.get(id);
    if (!p) return c;
    const a = Math.max(0, Math.min(1, alpha));
    return { x: p.x + (c.x - p.x) * a, y: p.y + (c.y - p.y) * a };
  }

  ids(): IterableIterator<number> {
    return this.curr.keys();
  }
}
