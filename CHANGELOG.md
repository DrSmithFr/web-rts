# Changelog

All notable changes to this project are documented here. Tags mark phase
merges (`vNN-phase-name`).

## Unreleased

## [v01-structure] - 2026-09-20

Phase 1: repository structure, build toolchain, WebAssembly bridge and the
empty deterministic simulation loop at 20 Hz.

### Added

- Single Go module (`rts`) shared by the native tools and the wasm entry:
  the simulation exists in exactly one copy under `sim/`.
- `sim/fixed`: signed 16.16 fixed-point arithmetic on int32 with frozen,
  tested edge behaviour (see below).
- `sim/rng`: canonical PCG32 (XSH-RR 64/32), one stream per domain
  (mapgen, combat, bot, misc); reference vectors pinned in
  `sim/rng/testdata/pcg32_vectors.txt` and matching the pcg_basic README
  sequence; regenerable only via `tools/genvec`.
- `sim/hash`: FNV-1a 64 state digest, explicit little-endian stream.
- `sim/proto`: compact binary order format (varint framed, no JSON).
- `sim/lockstep`: order queue with T+2 execution delay, stable sorted
  slices, no maps.
- `sim/core`: deterministic World with per-tick one draw per domain stream,
  order accounting and canonical `Hash()`/`HashHex()`.
- `tools/detcheck`: binary replay format (`RTSRPLY1`) and runner, committed
  scenario `tools/replays/phase1_noop.rts` (seed 0x5eed, 400 ticks, 11 noop
  orders, final hash `e5366f42a8df5b1c`).
- `tools/parity`: Node harness driving `sim.wasm` and line-exact native
  vs wasm stream comparison; `make test-parity` is green.
- `wasm`: js/wasm entry exposing `RTSSim` (newWorld/step/order/tick/hashHex/seed/version).
- `client`: Vite + TypeScript (strict) scaffold, wasm loader, `SimWorld`
  bridge, fixed-timestep loop (20 Hz simulation, capped catch-up), i18n
  skeleton with `en` as reference locale, HUD showing tick/hash/fps.
- `Makefile`: portable across sh and cmd.exe via node helpers; targets
  `vendor wasm test test-sim test-det test-parity test-client run-det dev fmt vet clean`.
- GitHub Actions matrix running `make test` on Linux, macOS (arm64) and
  Windows.

### Documented deviations from the specification

- `fixed.Div` by zero returns zero instead of failing; this keeps a bad
  division from killing a lockstep tick.
- Fixed-point arithmetic wraps around int32 instead of saturating.
- Reconnection replay, `T+2` network relay, hash arbitration on the wire,
  WebSocket functional tests and Playwright e2e are not before their phases
  (7 and 8); the lockstep API and the `make test` pipeline already reserve
  their slots (`test-func`, `test-e2e`).
- The CI workflow runs on push; this repository currently has no remote, so
  the first push activates it.
- Multi-platform hash parity is verified structurally (pure integer code,
  no floats/maps/time) and by the native-vs-wasm harness on the CI matrix;
  a third real-world OS cross-check happens once the remote exists.

## Phase map

1. structure, build, wasm bridge, 20 Hz loop — this release
2. map, terrain, camera, instanced rendering, selection
3. villagers, five resources, construction
4. pathfinding, avoidance, group orders, fog of war
5. combat, counters, production queues
6. era tree, technologies, victory conditions
7. lobby server, lockstep network, desync detection, reconnection
8. bots, difficulty, balance, final menus
