#!/usr/bin/env node
// Cross-platform filesystem helpers for the Makefile, because cmd.exe has
// no cp, mkdir -p or rm -rf.
//
//   node tools/fsutil.mjs copy <src> <dst>
//   node tools/fsutil.mjs mkdir <dir>
//   node tools/fsutil.mjs rmrf <path>...
import fs from 'node:fs';
import path from 'node:path';

const [cmd, ...args] = process.argv.slice(2);

switch (cmd) {
  case 'copy': {
    const [src, dst] = args;
    fs.mkdirSync(path.dirname(dst), { recursive: true, force: true });
    fs.copyFileSync(src, dst);
    break;
  }
  case 'mkdir': {
    for (const dir of args) {
      fs.mkdirSync(dir, { recursive: true, force: true });
    }
    break;
  }
  case 'rmrf': {
    for (const target of args) {
      fs.rmSync(target, { recursive: true, force: true });
    }
    break;
  }
  default:
    console.error(`unknown fsutil command: ${cmd}`);
    process.exit(2);
}
