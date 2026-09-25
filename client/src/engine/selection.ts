// Selection state for map features and (later) units. The store is pure;
// the rectangle picker takes already-projected screen coordinates so both
// are testable without a renderer.

export const MAX_SELECTION = 64;

export class Selection {
  private ids: number[] = [];

  get size(): number {
    return this.ids.length;
  }

  has(id: number): boolean {
    return this.ids.includes(id);
  }

  toggle(id: number): void {
    const i = this.ids.indexOf(id);
    if (i >= 0) {
      this.ids.splice(i, 1);
    } else if (this.ids.length < MAX_SELECTION) {
      this.ids.push(id);
    }
  }

  replace(ids: number[]): void {
    this.ids = ids.slice(0, MAX_SELECTION);
  }

  clear(): void {
    this.ids = [];
  }

  toArray(): number[] {
    return [...this.ids];
  }
}

export interface ScreenPoint {
  id: number;
  sx: number;
  sy: number;
  // syTop is the screen y of the top of the entity's body: a box drag
  // over the model (not just its feet) still selects it.
  syTop?: number;
  onScreen: boolean;
}

export interface Rect {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

export function normalizeRect(x0: number, y0: number, x1: number, y1: number): Rect {
  return {
    x0: Math.min(x0, x1),
    y0: Math.min(y0, y1),
    x1: Math.max(x0, x1),
    y1: Math.max(y0, y1),
  };
}

// selectInRect returns every on-screen point id inside the rectangle,
// inclusive on all edges, in input order, capped at MAX_SELECTION.
export function selectInRect(points: ScreenPoint[], rect: Rect): number[] {
  const out: number[] = [];
  for (const p of points) {
    if (!p.onScreen) continue;
    const inFoot = p.sx >= rect.x0 && p.sx <= rect.x1 && p.sy >= rect.y0 && p.sy <= rect.y1;
    const inTop =
      p.syTop !== undefined && p.syTop >= rect.y0 && p.syTop <= rect.y1 && p.sx >= rect.x0 && p.sx <= rect.x1;
    if (inFoot || inTop) {
      out.push(p.id);
      if (out.length >= MAX_SELECTION) break;
    }
  }
  return out;
}
