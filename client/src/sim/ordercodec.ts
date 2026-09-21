// TypeScript mirror of sim/orders: builds the exact binary payloads the
// simulation decodes. The byte format is pinned by
// sim/orders/testdata/order_vectors.txt, exercised by ordercodec.test.ts.

export const OrderKind = {
  Move: 1,
  Gather: 2,
  Build: 3,
  Stop: 4,
  Train: 5,
  MoveGroup: 6,
} as const;

export interface MoveItem {
  id: number;
  x16: number;
  y16: number;
}

class Writer {
  private bytes: number[] = [];

  uvarint(v: number): void {
    let x = Math.floor(v);
    while (x >= 0x80) {
      this.bytes.push((x & 0x7f) | 0x80);
      x = Math.floor(x / 128);
    }
    this.bytes.push(x);
  }

  u16(v: number): void {
    this.bytes.push(v & 0xff, (v >>> 8) & 0xff);
  }

  out(): Uint8Array {
    return new Uint8Array(this.bytes);
  }
}

export function movePayload(items: MoveItem[]): Uint8Array {
  const w = new Writer();
  w.uvarint(items.length);
  for (const it of items) {
    w.uvarint(it.id);
    w.u16(it.x16);
    w.u16(it.y16);
  }
  return w.out();
}

export function gatherPayload(unit: number, node: number): Uint8Array {
  const w = new Writer();
  w.uvarint(unit);
  w.uvarint(node);
  return w.out();
}

export function buildPayload(unit: number, def: number, x: number, y: number): Uint8Array {
  const w = new Writer();
  w.uvarint(unit);
  w.u16(def);
  w.u16(x);
  w.u16(y);
  return w.out();
}

export function stopPayload(ids: number[]): Uint8Array {
  const w = new Writer();
  w.uvarint(ids.length);
  for (const id of ids) w.uvarint(id);
  return w.out();
}

export function trainPayload(building: number): Uint8Array {
  const w = new Writer();
  w.uvarint(building);
  return w.out();
}

// moveGroupPayload: group move to one base tile; the sim assigns
// formation slots deterministically in id order.
export function moveGroupPayload(ids: number[], x16: number, y16: number): Uint8Array {
  const w = new Writer();
  w.uvarint(ids.length);
  for (const id of ids) w.uvarint(id);
  w.u16(x16);
  w.u16(y16);
  return w.out();
}

// Reader decodes payloads (used by tests and the debug console).
export class PayloadReader {
  private pos = 0;
  ok = true;

  constructor(private readonly src: Uint8Array) {}

  uvarint(): number {
    let v = 0;
    let shift = 0;
    for (let i = 0; i < 10; i++) {
      if (this.pos >= this.src.length) {
        this.ok = false;
        return 0;
      }
      const b = this.src[this.pos++];
      v += (b & 0x7f) * 2 ** shift;
      if (b < 0x80) return v;
      shift += 7;
    }
    this.ok = false;
    return 0;
  }

  u16(): number {
    if (this.pos + 2 > this.src.length) {
      this.ok = false;
      return 0;
    }
    const v = this.src[this.pos] | (this.src[this.pos + 1] << 8);
    this.pos += 2;
    return v;
  }
}
