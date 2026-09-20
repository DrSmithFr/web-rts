#!/usr/bin/env node
// Parity harness: replays a scenario file through the WebAssembly build of
// the simulation, printing the same checkpoint stream as tools/detcheck.
// make test-parity diffs the two outputs; any difference is a determinism
// bug between native and wasm builds.
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..', '..');
const staticDir = path.join(root, 'client', 'static');
const replayPath =
  process.argv[2] ?? path.join(root, 'tools', 'replays', 'phase1_noop.rts');
const HASH_PERIOD = 20n;

function parseReplay(buf) {
  const magic = buf.subarray(0, 8).toString('ascii');
  if (magic !== 'RTSRPLY1') {
    throw new Error(`bad magic: ${magic}`);
  }
  const seed = buf.readBigUInt64LE(8);
  if (seed > 9007199254740991n) {
    throw new Error('seed larger than 2^53 is not supported across the bridge');
  }
  const ticks = buf.readUInt32LE(16);
  const orderCount = buf.readUInt32LE(20);
  let p = 24;
  const orders = [];
  for (let i = 0; i < orderCount; i++) {
    const tick = buf.readUInt32LE(p);
    const player = buf[p + 4];
    const kind = buf.readUInt16LE(p + 5);
    p += 7;
    let len = 0n;
    let shift = 0n;
    for (;;) {
      const b = buf[p++];
      len |= BigInt(b & 0x7f) << shift;
      if ((b & 0x80) === 0) break;
      shift += 7n;
    }
    const payload = new Uint8Array(buf.subarray(p, p + Number(len)));
    p += Number(len);
    orders.push({ tick, player, kind, payload });
  }
  return { seed: Number(seed), ticks, orders };
}

const wasmExecSrc = fs.readFileSync(path.join(staticDir, 'wasm_exec.js'), 'utf8');
vm.runInThisContext(wasmExecSrc, { filename: 'wasm_exec.js' });

const go = new globalThis.Go();
const bytes = fs.readFileSync(path.join(staticDir, 'sim.wasm'));
const mod = await WebAssembly.compile(bytes);
const instance = new WebAssembly.Instance(mod, go.importObject);
go.run(instance);

// exports.run executes the Go main synchronously inside go.run, but poll a
// few macrotasks for robustness against runtime scheduling changes.
let sim = globalThis.RTSSim;
for (let i = 0; i < 100 && sim === undefined; i++) {
  await new Promise((resolve) => setTimeout(resolve, 10));
  sim = globalThis.RTSSim;
}
if (sim === undefined) {
  throw new Error('RTSSim bridge did not register');
}

const replay = parseReplay(fs.readFileSync(replayPath));
const world = sim.newWorld(replay.seed);
let oi = 0;
for (let t = 0; t < replay.ticks; t++) {
  while (oi < replay.orders.length && replay.orders[oi].tick <= t) {
    const o = replay.orders[oi];
    sim.order(world, o.player, o.kind, o.payload);
    oi++;
  }
  sim.step(world);
  if ((t + 1) % Number(HASH_PERIOD) === 0) {
    console.log(`tick=${t + 1} hash=${sim.hashHex(world)}`);
  }
}
console.log(`final=${sim.hashHex(world)}`);
process.exit(0);
