# Changelog

All notable changes to this project are documented here. Tags mark phase
merges (`vNN-phase-name`).

## Unreleased

## [v07-network-lobby] - 2026-09-22

Phase 7: authoritative multiplayer server and browser session client.

### Added

- `server` package + `rts-server` binary: lobby creation with 6-character
  invitation codes, up to eight human/bot slots, host-only start, and
  `-fast` accelerated tick loop (`make run-server`).
- WebSocket protocol (`server/wire`, JSON frames with base64 order
  payloads): create/join/start/order/hash plus welcome/lobby/start/tick/
  catchup/pause/resume/desync events; seeds and hashes travel as hex
  strings to survive JSON number precision.
- Lockstep relay stamped `T+2` over `sim/lockstep`: every tick broadcasts
  the exact order batch the authoritative world stepped with plus the
  state hash; clients step from the batches and confirm hashes back.
- Arbitration and flow control: mismatched client hashes flag the player
  and broadcast a desync event; a client four hash windows behind pauses
  the room and resumes on catch-up.
- Reconnect via welcome token: the slot is reclaimed and the seed plus
  full order log replay the client up deterministically.
- `client/src/net/session.ts`: session client with boot/catch-up replay,
  per-tick stepping through the new `stepWith` wasm export, hash
  verification and confirmation, tick buffering during boot; URL entry
  points (`?ws=...&create=1[&bots=N][&autostart=1]`, `?ws=...&code=...`),
  net-status banner, orders routed through the socket in online mode and
  `LOCAL_PLAYER` taken from the server.
- `wasm`: `newWorld(seed, players)` and `stepWith(id, batch)` exports.
- Functional tests (`go test ./server/`, wired into `make test-func`):
  lobby create/join/leave/start, T+2 relay discipline, induced desync,
  reconnect with replay catch-up, and a scripted game against three bots
  driven to relic victory at accelerated speed.

### Changed

- `make test-func` now runs the WebSocket scenarios instead of skipping.

## [v06-eras-tech-victory] - 2026-09-21

Phase 6: the five-era ladder with age-up, prerequisite technology trees,
map relics and the three victory conditions.

### Added

- `sim/era` + `data/eras.json`: Prehistory .. Modern ladder with age-up
  costs, prerequisite buildings and research time; validation tests.
- `sim/tech` + `data/techs.json`: 15 technologies with era gates,
  research buildings, prerequisite chains and flat integer stat effects
  (damage, armor, speed, carry, range) folded into per-player modifier
  sets; validation and mod tests.
- Core: age-up and research orders (`AgeUp` 9, `Research` 10 with pinned
  wire vectors), per-player era/tech state in the world hash
  (`rts.world/v6`), era gates on building and training, five map relics
  claimed by standing on them (`rts.map/v4`), conquest / Wonder / relic
  victory resolution with a first-settled winner; 8 new core tests.
- Data: Market, Forge, Bank and Wonder buildings; Knight and Crossbowman
  units (Medieval); Antiquity gates on the military trio; era costs
  rebalanced to the real gather economy (food front-loaded, gold scaling).
- `wasm`: 40-byte economy records (era, age progress, current research),
  `eras`, `techDefs`, `playerTechs`, `victory`, `relicsBlob` exports.
- `client`: era + relic counters in the resource bar, age-up button with
  cost/requirement states and aging progress, per-building research
  buttons with prerequisite and affordability gating, research progress
  line, golden relic markers on the minimap, victory banner; i18n keys
  for eras, techs and victory.
- `detcheck`: staged recorder (goals fire off live probe state, replay
  stays reproducible); new `eras` scenario covering age-up, market,
  lumber, a two-tech prerequisite chain and lancer training.

### Changed

- Replays regenerated for `rts.world/v6`; new `phase6_eras.rts`
  (12 000 ticks) recorded, verified and parity-checked;
  `make test-det`/`test-parity` now gate five replays.
- Building defs carry `minEra`; the build menu hides buildings above the
  player's era.

## [v05-combat-hunting-control-groups] - 2026-09-21

Phase 5: the combat triangle, huntable wildlife, attack/patrol orders,
control groups and real production queues.

### Added

- `sim/units`: unit definition table (`data/units.json`) for villager,
  lancer, archer, scout, deer and boar: classes, damage types, armor,
  +10 counter bonuses, costs, train buildings, corpse food, flee flag;
  strict float-free parsing with validation tests.
- Core: military units (Attack/Patrol/Retaliate/Auto-acquire states,
  cooldowns, ranged reach via `range16`), wildlife (deer flee, boar
  retaliate), death processing with pop release and animal corpses
  becoming harvestable food nodes; neutral owner (`0xFF`); per-def
  speeds and vision; world hash `rts.world/v5`.
- `sim/worldgen`: animals placed around each spawn (3 deer + 2 boar);
  map digest `rts.map/v3`.
- Orders: `Attack` (7) and `Patrol` (8) with pinned wire vectors; `Train`
  payload gains an optional unit-def id (old replays still decode).
- `wasm`: 24-byte unit records (def, target, hp), 30-byte building
  records (train queue), `unitDefs(id)` export.
- `client`: class-aware unit rendering (beast bodies, lancer spear,
  scout mount, health tint, neutral animal colors); right-click attacks
  enemies and wildlife, shift+right-click patrols; control groups
  (Ctrl+digit binds, digit recalls, Shift adds — `engine/groups.ts`);
  per-unit train buttons with costs, live training progress and queue
  count in the panel; barracks building; minimap shows neutral animals.

### Changed

- Replays regenerated for `rts.world/v5`; new `phase5_combat.rts`
  scenario (deer hunt, barracks on both bases, stalled training
  queues, patrol into enemy territory, cross-map attack) recorded,
  verified and parity-checked; `make test-det`/`test-parity` now gate
  four replays.
- Training moved from the building def's single villager slot to the
  per-unit queue (`TrainCur`/`TrainQueue`).

## [v04-pathfinding-fog-interpolation] - 2026-09-21

Phase 4: flow-field pathfinding with formations, per-player three-state
fog of war, and 60 fps position interpolation between 20 Hz ticks.

### Added

- `sim/path`: deterministic BFS flow fields with no corner cutting,
  fixed tie-break order and a 64-slot spiral formation table; unit tests
  on hand-drawn ASCII maps (corridors, lakes, diagonal gaps, waits).
- `sim/fog`: three-state (unexplored / explored / visible) per-player
  fog with radius vision (villager 6, building 8, site 4), decay to
  explored, 2-bit packed wire format; part of the world hash
  (`rts.world/v4`).
- Core: group move order (`MoveGroup`) with shared flow field and
  deterministic formation slots; buildings and sites now block movement
  through an occupancy grid; unreachable moves wait instead of sliding;
  arrival snaps exactly onto the formation slot.
- Bridge: `fogBlob(id, player)`.
- `client`: chunked ground (32-tile meshes, frustum culling, per-chunk
  fog repaint); fog hides nodes and enemy entities and dims explored
  ground; units interpolate between ticks (`Interpolated` + loop alpha)
  with a rigid walking bob; clickable canvas minimap with fog and
  camera box; right-click now issues one formation group move.
- Spec alignment: deterministic 1000x1000 map generation pinned by a
  test (~150 ms, 250k nodes); oil stays invisible until the Modern era.

### Changed

- Replays regenerated for `rts.world/v4`; new `phase4_paths.rts`
  scenario (group move across terrain, second move, fog reveal) is
  recorded, verified and parity-checked; `make test-det` now gates three
  replays.

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
