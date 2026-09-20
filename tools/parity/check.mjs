#!/usr/bin/env node
// Line-exact comparison of the native and wasm checkpoint streams. Prints
// the first mismatch and exits 1 on divergence, so make never retries.
import fs from 'node:fs';

const [fileA, fileB] = process.argv.slice(2);
const a = fs.readFileSync(fileA, 'utf8').replace(/\n$/, '').split('\n');
const b = fs.readFileSync(fileB, 'utf8').replace(/\n$/, '').split('\n');

if (a.length !== b.length) {
  console.error(`parity mismatch: ${fileA} has ${a.length} lines, ${fileB} has ${b.length}`);
}
const n = Math.min(a.length, b.length);
for (let i = 0; i < n; i++) {
  if (a[i] !== b[i]) {
    console.error(`parity mismatch at line ${i + 1}:\n  native: ${a[i]}\n  wasm:   ${b[i]}`);
    process.exit(1);
  }
}
console.log(`parity ok: native == wasm over ${a.length} lines`);
process.exit(a.length === b.length ? 0 : 1);
