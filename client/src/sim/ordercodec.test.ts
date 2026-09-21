import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import {
  buildPayload,
  gatherPayload,
  movePayload,
  stopPayload,
  trainPayload,
  PayloadReader,
} from './ordercodec';

// The shared Go/TS vector file pins the wire format on both sides.
const vectorsUrl = new URL('../../../sim/orders/testdata/order_vectors.txt', import.meta.url);

function hexOf(b: Uint8Array): string {
  return [...b].map((x) => x.toString(16).padStart(2, '0')).join('');
}

interface Line {
  kind: string;
  args: string;
  hex: string;
}

function lines(): Line[] {
  return readFileSync(vectorsUrl, 'utf8')
    .trim()
    .split('\n')
    .map((l) => {
      const [kind, args, hex] = l.split('|');
      return { kind, args, hex };
    });
}

function encode(kind: string, args: string): Uint8Array {
  switch (kind) {
    case 'move': {
      const items = args === '' ? [] : args.split(';').map((p) => {
        const [id, xy] = p.split(':');
        const [x, y] = xy.split(',');
        return { id: Number(id), x16: Number(x), y16: Number(y) };
      });
      return movePayload(items);
    }
    case 'gather': {
      const [u, n] = args.split(':').map(Number);
      return gatherPayload(u, n);
    }
    case 'build': {
      const [u, d, x, y] = args.split(':').map(Number);
      return buildPayload(u, d, x, y);
    }
    case 'stop':
      return stopPayload(args.split(';').map(Number));
    case 'train':
      return trainPayload(Number(args));
    default:
      throw new Error(`unknown kind ${kind}`);
  }
}

describe('order codec matches shared vectors', () => {
  for (const v of lines()) {
    it(`${v.kind} ${v.args}`, () => {
      expect(hexOf(encode(v.kind, v.args))).toBe(v.hex);
    });
  }
});

describe('payload reader decodes vectors', () => {
  it('move items roundtrip', () => {
    const raw = movePayload([
      { id: 7, x16: 320, y16: 160 },
      { id: 8, x16: 321, y16: 161 },
    ]);
    const r = new PayloadReader(raw);
    expect(r.uvarint()).toBe(2);
    expect([r.uvarint(), r.u16(), r.u16()]).toEqual([7, 320, 160]);
    expect([r.uvarint(), r.u16(), r.u16()]).toEqual([8, 321, 161]);
    expect(r.ok).toBe(true);
  });

  it('flags truncated payloads', () => {
    const r = new PayloadReader(new Uint8Array([1]));
    r.u16();
    expect(r.ok).toBe(false);
  });
});
