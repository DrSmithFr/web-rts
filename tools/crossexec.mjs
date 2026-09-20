#!/usr/bin/env node
// Runs a command with extra environment variables, portable across sh and
// cmd.exe (which has no inline `KEY=VAL cmd` syntax).
//
//   node tools/crossexec.mjs KEY=VAL [KEY=VAL...] -- cmd [args...]
import { spawnSync } from 'node:child_process';

const sep = process.argv.indexOf('--');
if (sep < 0) {
  console.error('crossexec: missing --');
  process.exit(2);
}
const env = { ...process.env };
for (const pair of process.argv.slice(2, sep)) {
  const i = pair.indexOf('=');
  if (i <= 0) {
    console.error(`crossexec: bad assignment: ${pair}`);
    process.exit(2);
  }
  env[pair.slice(0, i)] = pair.slice(i + 1);
}
const [cmd, ...args] = process.argv.slice(sep + 1);
const result = spawnSync(cmd, args, { env, stdio: 'inherit', shell: false });
if (result.error) {
  console.error(result.error);
  process.exit(1);
}
process.exit(result.status ?? 1);
