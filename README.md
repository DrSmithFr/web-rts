# rts

A deterministic browser RTS inspired by Age of Empires, rendered with Three.js
and simulated in Go compiled to WebAssembly. The same simulation package runs
natively on the server and inside every browser client via lockstep
determinism.

## Layout

```
sim/      deterministic simulation, pure Go (no float, no map iteration, no time, no goroutines)
server/   lobby, order relay, arbitration, bots (phase 7)
wasm/     WebAssembly entry point and JavaScript bridge
client/   TypeScript, Three.js rendering, UI, inputs (Vite)
data/     balance data for units, buildings, technologies (loaded, never hard-coded)
tools/    replay format, determinism checker, parity harness
```

## Determinism contract

- Fixed-point 16.16 arithmetic on `int32` (`sim/fixed`). No `float` in `sim/`.
- PCG32 PRNG with explicit seeds, one stream per domain (`sim/rng`).
- Entities live in slices with stable integer ids; never iterate Go maps in
  simulation code.
- No system clock, no `time.Now`, no goroutines inside `sim/`.
- State is hashed with FNV-1a 64 (`sim/hash`), every 20 ticks on the wire.
- Orders carry absolute due ticks; clients apply them at `T+2`
  (`sim/lockstep`).
- Simulation runs at 20 Hz; rendering interpolates at 60 fps client-side.

## Network model (phase 7)

Clients send orders, never state. The server stamps each order for tick `T+2`
and rebroadcasts it to all players. Every 20 ticks each client sends its state
hash; the server compares, flags desyncs and dumps differential state.

## Build

Requirements: Go >= 1.27, Node >= 22, GNU make.

```sh
make vendor      # copy wasm_exec.js from the Go toolchain
make wasm        # build client/static/sim.wasm
make test        # sim unit tests + replay + native/wasm parity + client tests
make run-det     # replay the committed scenario natively, print hashes
make dev         # serve the client on http://localhost:5173
```

## Status

Phase 1: repository structure, build toolchain, WebAssembly bridge and the
empty 20 Hz simulation loop shared natively and in the browser. See
`CHANGELOG.md`.
