# Changelog

All notable changes to this project are documented here. Tags mark phase
merges (`vNN-phase-name`).

## Unreleased

## [v03-villagers-resources-construction] - 2026-09-21

Phase 3: villagers harvest five resources, build the first buildings and
train new villagers — the economy loop is playable end to end.

### Added

- `sim/entity`: generic slot store with generation-carrying ids
  (`gen<<16|slot`), free-list reuse and slot-ordered iteration.
- `sim/building` + `data/buildings.json`: validated balance table
  (hut, campfire, storehouse) embedded at the module root; all
  gameplay numbers live in data, parsed once, never on the hot path.
- `sim/resource`: the five resources; worldgen now places harvestable
  nodes (tree per forest tile, stone deposit per rock tile, sparse gold
  and oil, four guaranteed berry bushes per spawn via a deterministic
  offset ladder) and folds them into the map digest (`rts.map/v2`).
- `sim/orders`: binary payloads for move/gather/build/stop/train,
  byte-for-byte pinned by `testdata/order_vectors.txt` and mirrored in
  `client/src/sim/ordercodec.ts` against the same file.
- `sim/core`: economy simulation — starting resources (100w/50s/200f/50g),
  three villagers plus a storehouse per spawn, villager state machine
  (idle/move/gather/return/build), carry-10 trips to the nearest dropoff,
  construction sites built by escorted villagers, population cap
  (5 + hut rooms) and storehouse villager training (50 food, 20 ticks).
  Full state hashed (`rts.world/v3`); fixed 16.16 movement is direct-line
  (pathfinding lands in phase 4).
- Bridge: `unitsBlob/nodesBlob/buildingsBlob/economyBlob/defs` compact
  little-endian records; typed decoding in `SimWorld`.
- `client`: villagers rendered owner-colored and instanced; nodes shrink
  visually as they deplete; buildings grow with build progress. Top
  resource bar (wood/stone/food/gold/pop), selection panel with build
  buttons and placement ghost (esc cancels), right-click moves or
  gathers, training from the storehouse panel.

### Changed

- Deterministic replays: `tools/detcheck -scenario economy` records a
  scripted gather/build/train run (`tools/replays/phase3_economy.rts`);
  both replays are gated by `make test-det` and native↔wasm parity.

## [v02-map-camera-selection] - 2026-09-21

Phase 2: procedural map, Three.js low-poly rendering, RTS camera and
click/rectangle selection of map features.

### Added

- `sim/terrain`: six terrain types and the tile grid with whole-number
  vertex heights (slice-based, row-major).
- `sim/worldgen`: deterministic generator — splitmix64-hashed value noise
  in 16.16 fixed point, integer classification thresholds tuned over 200+
  seeds (grass ~33%, forest ~23%, sand ~14%, water ~28%, rock ~2%), and
  farthest-point spawn placement with a minimum 48-tile separation.
- Balance guarantee: every spawn gets an identical starting area — grass
  disk, forest ring of eight tiles, rock corners at fixed offsets — all on
  land (tested over 40 seeds). The map digest is part of the world hash
  (`rts.world/v2`); phase-1 replay regenerated.
- `tools/mapstat`: threshold tuning statistics for the generator.
- Bridge: `mapSize/mapTiles/mapHeights/mapHashHex/spawnCount/spawnAt`, and
  typed `SimWorld` accessors decoding u16 vertex heights.
- `client`: Three.js rendering — flat-shaded single-draw-call ground mesh,
  one InstancedMesh per object kind (trees, rocks, spawn markers) under
  one directional light with a shadow map, gradient sky, fog.
- RTS camera: yaw orbit at fixed pitch, zoom, WASD/arrow pan relative to
  yaw, Q/E rotate, edge scroll, right-drag rotate, clamped to map bounds.
- Selection: click a feature or drag a screen rectangle; white rings mark
  selection (cap 64); selection store and rectangle picker are pure and
  unit tested (14 client tests now).
- Dependencies added: `three` (MIT, rendering) and `@types/three` (MIT).

### Fixed

- `sim/worldgen`: `[4]int32` literal with three initializers silently
  injected a zero height into the slope computation (found via headless
  browser parity of map hashes between native and wasm).

### Notes

- Native and wasm map hashes match for the same seed (verified headless
  in chromium against `tools/detcheck` output).

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
