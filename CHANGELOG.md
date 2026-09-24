# Changelog

All notable changes to this project are documented here. Tags mark phase
merges (`vNN-phase-name`).

## Unreleased

Phase 10: procedural map generation spec — biomes, tree species, sawmill
gating and biome-driven agriculture. Simulation-affecting: map format
`rts.map/v4`, world `rts.world/v7`, replays regenerated.

### Added

- `data/trees.json` and `data/biomes.json` (embedded in `assets.go`):
  chop speed, wood/food yields and sawmill requirement per species;
  agriculture multiplier and dominant tree per biome — all tunable
  without touching code (W4, H4).
- `sim/tree` and `sim/biome`: validated balance tables with
  deterministic ids mirrored client-side (`TREE`, `BIOME` in palette).
- Worldgen biome layer (`sim/worldgen`): temperature latitude ramp +
  noise, moisture and a distance-to-water flood classify every tile;
  regions are contiguous with gradual borders and a cold-north /
  warm-south fallback guarantees at least three biomes (B1..B4).
- Trees grow in their biome: taiga spruce, temperate and prairie birch,
  savanna baobab (forest tiles plus scattered individuals), coast palms
  on sand (food only). Forest noise punches clearings (B5); every
  player's starter ring is fast-chop birch.
- Mountain mask: rock only falls inside localized noise massifs, so
  mountains cluster instead of speckling (T2), and a land mask lifts the
  map rim so one walkable component dominates.
- Spawns are flat (vertex spread checked, disk averaged at stamping,
  T3) and every spawn is chosen inside the largest land component, so
  one player reaches every other (T4, C2).
- Species-chop speed in `sim/core` (fixed-point `GatherAcc` remainder),
  sawmill gate on baobab chops (orders refused and in-progress gather
  dropped, W3), farms (`farm` building, id 10) producing food at the
  biome multiplier of their tile (H1, H2, C7).
- `sawmill` (id 9) and `farm` (id 10) buildings in `data/buildings.json`.
- Gather chaining (`sim/core`): when a node runs dry, the gatherer
  auto-retargets the nearest still-harvestable node in vision — same
  resource kind and same tree species first, then same kind — chaining
  trees, bushes, stone and gold without new orders (sawmill gating
  respected). Finished carcasses chain back to live, harmless prey
  (deer; boars are never auto-attacked), tracked by a new `Corpse` node
  flag; world hash v8, replays re-recorded.
- Bridge v0.2.0: `mapBiomes(id)`, `biomeDefs(id)`, `treeDefs(id)`, tree
  byte in node records.
- Client: ground and minimap tinted per biome (B1), one silhouette and
  palette entry per tree species so species reads before a chop (W1),
  hover shows biome + farm multiplier and tree names with a
  "sawmill required" hint (H3, W3); i18n names for biomes, trees and
  the two new buildings.
- Fixed: order rings now sit on the terrain surface instead of sinking
  under raised ground; right-click gather picks the node nearest the
  cursor on screen instead of the first one in a fixed 3×3 scan order.
- Attack orders now name the target store (units vs buildings) with a
  `TargetKind` wire byte and a hashed `Unit.AttackKind` (world v8→v9):
  soldiers can raz enemy buildings (new right-click on an enemy
  building), while gather orders are villager-only — soldiers no longer
  harvest.
- New `Repair` order (right-click on your own building under
  construction) reassigns villagers to finish an interrupted
  construction.
- Procedural audio (Tone.js, no samples): `client/src/audio` runs a
  seeded music engine — fixed tonic and minor scale per match, chord
  progression in 4/8-bar blocks, a mutating 2-bar motif — with four
  capped synth layers (pad, bass, melody, percussion). The game feeds
  it a mood (`calm`/`tension`/`combat`, applied at the next bar with
  gain fades and tempo ramps) and the biome under the camera (timbre
  only). Victory and defeat themes close the match. Procedural sound
  effects cover selection, orders, hits, deaths, placement, completed
  buildings and age-ups; music and effects volumes are separate and
  persisted, adjustable from the Options screen.
- Day/night cycle: one 8-minute cycle (9600 ticks) derived purely from
  the sim tick — no world state, hash or replay impact. Canonical
  `sim/day` package (phase, solar elevation, night check) mirrored
  client-side with matching tests. Rendering rotates the sun direction,
  fades to a pale moon after dusk, crossfades hemisphere fill, fog and
  sky gradient between warm day, golden dusk and blue night palettes,
  and shifts tone-mapping exposure. Ready for behavior systems to
  consume the phase. Campfires are real light sources: each standing
  campfire gets an animated flame, and the nearest ones (pool of 8
  point lights) cast a warm flickering glow that strengthens with the
  night.

### Fixed

- Nodes (trees, ore, berries) are placed with the bilinear ground sample
  at their center instead of the tile-corner height, so they never float
  above or sink into sloped terrain — visible even on stumps.
- Starting storehouses now clear the resource nodes under their footprint
  (`clearNodesUnder`); bases never spawn on top of trees or berries.
  World hash `rts.world/v9` → `v10`, all five replays re-recorded.
- Selection no longer confuses a unit and a building with the same entity
  id (the stores count independently): building selection keys carry a
  client-side offset, rings and panels highlight exactly one kind, and
  order payloads (stop/attack/repair) always send raw ids.
- Placement ghost is the translucent building itself (real materials) on
  a footprint overlay that turns red over nodes, buildings or the map
  border.
- Panel costs reuse the resbar icons (wood/stone/food/gold) via shared
  SVG symbols; depleted nodes darken and shrink, trees leave stumps;
  harvesting pops a floating `+N` in the resource color; hovering a unit
  shows its name and HP; unit silhouettes, contact shadows, rings and
  HP bars scale with camera distance; minimap shows a dim terrain preview
  of unexplored ground plus node and building dots; zoom-out is clamped
  closer and the pan bounds tightened.
- Panel buttons survive progress refreshes: the panel HTML is rebuilt only
  when its structural signature changes, progress bars update in place and
  a single delegated click handler serves all buttons — clicks no longer
  vanish mid-rebuild. Click-vs-drag threshold widened to 12 px (jittery
  clicks select instead of starting a micro rect-drag that missed the
  unit's foot point), right-drag camera rotation no longer issues a
  move/attack order, and right-clicks begun on the HUD never leak a world
  order.   Hover chip no longer intercepts clicks.
- Clicking an entity's model now selects it: units, animals and buildings
  are picked by the screen segment from their feet to their model top
  (heights measured from the render geometry bounding boxes), with the
  radius growing as the model looks bigger on screen — roofs and heads
  are clickable, not just the ground tile under the feet. Box-select
  and enemy attack-target picking use the same body segments.

### Tests

- `sim/tree`, `sim/biome` table tests; core tests pin chop-speed order,
  palm food-only yields, baobab sawmill gating and farm multipliers per
  biome (C6, C7).
- `worldgen`: acceptance test over 20 seeds (plains majority, flat and
  connected spawns, >=3 biomes, all four species present in-biome) plus
  a biome contiguity check (C1..C5).
- `mapstat -accept N` exposes the same gate as a tool; five replays
  re-recorded and verified on `rts.world/v7`, native/wasm parity holds.
- Playwright: e2e/perf specs retry once (two attempts maximum per the
  testing policy).
- Vitest: audio generation specs (seed reproducibility, diatonic-only
  progressions, motif bounds and mutation size, mood/layer table).

### Notes

- Fixed a Makefile staleness trap: `bin/rts-server` now depends on
  `sim/` sources.
- E2E specs gather berries explicitly (`tree === 0`) since palms are
  food nodes too.

## [v09-graphics] - 2026-09-23

Phase 9: the graphics pass — code-generated low-poly models, rigid-part
animation, terrain art, combat effects, lighting and a UI redesign.
Client-side only: the simulation byte stream is untouched, all effect
state derives from snapshot deltas and interpolated positions.

### Added

- `client/src/render/models.ts`: unit and building models authored as
  painted primitives merged into per-def geometry, split into a static
  body, a player-tinted insignia mesh and named animated parts whose
  joints sit at the geometry origin. Budget: ≤300 triangles per model
  (asserted by tests).
- `client/src/render/animate.ts`: pure animation math — shortest-angle
  yaw stepping, distance-driven walk phase, eased swing amplitude.
- `client/src/render/hash.ts`: stable integer hash for decor scatter
  and per-instance jitter.
- Actors rebuilt (`actors.ts`): eight distinct units with facing from
  movement, target-facing in fight/gather/build states, leg/arm/weapon
  cycles, villagers visibly carry the gathered resource kind, HP bars
  billboard over damaged units, pulsing selection torus.
- Buildings rebuilt (`buildings.ts`): eight distinct silhouettes with
  player insignia parts, foundation/scaffold/rising-core construction
  visuals, flickering campfire flames.
- Terrain art (`terrainMesh.ts`, `water.ts`, `scatter.ts`): per-tile
  brightness jitter, rock color on steep slopes, sand shoreline blending,
  translucent animated water surface (shared-`uTime` shader ripple),
  deterministic decor scatter — canopy trees on forest tiles, tufts,
  flowers, pebbles, shore reeds.
- `client/src/render/fx.ts`: additive instanced spark bursts (hit,
  death, harvest pops) and expanding ground order rings.
- Lighting and camera: ACES tone mapping, re-tuned sky/fog, shadow
  frustum follows the view (`frameShadow`), zoom/pan/yaw easing in
  `RtsCamera`.
- UI redesign in `index.html` + panel DOM: brass/ink theme with blurred
  panels, SVG resource chips with change-flash, era pips, progress bars
  for training/research/aging/building, framed minimap moved bottom-left,
  victory/net-status animations.
- i18n: added the missing unit/building names and `res.pop`,
  `res.relics` keys; resource chip tooltips are localized.

### Tests

- New Vitest tables for `hash`, `animate` and the model triangle budget
  (30 new tests; client suite now 77).
- `make test` green end to end; 500-unit E2E perf spec holds.

### Notes

- Palette (`palette.ts`) formalized as the curated ~26-color set;
  decor, material and fire entries added, no hues outside the file.
- `CHROMIUM=/usr/bin/chromium` must be set for `make test-e2e` when the
  bundled Playwright browsers are not installed.

## [v08-bots-menus-e2e] - 2026-09-22

Phase 8: deterministic bot AI, game menus, and Playwright end-to-end tests.

### Added

- `sim/bots`: fog-limited, fully deterministic `Brain` state machine —
  villager allocation, compass-spiral exploration, wood/deer seeking,
  per-era build order, age-ups, tech research, military production and
  a relic rush. Easy/Standard/Hard alter only reaction speed and
  aggression. Food is finite on the map (berries + deer), so doctrine
  keeps villages lean; unsupervised 3-bot games run to a relic
  victory.
- Brain tests: era ladder, brain determinism (identical hashes across
  runs), three-brain game to winner.
- Server-hosted bots: `start` carries `difficulty`
  (easy/standard/hard; empty leaves slots idle for scripted tests);
  brains step inside the authoritative loop and their orders join the
  per-tick batch, the order log, catch-up and replays.
  `TestBotsHostedOnServer` hash-matches a mirror client for 3000 ticks.
- Solo play: wasm `brainNew` export; `?bots=N&difficulty=...` runs
  bots entirely in the browser sim.
- Main menu and game-setup screens (solo/multiplayer/options/credits;
  seed, map, bot count, difficulty, victory fields); boots remain
  fully URL-driven (`menu=off` bypasses), which keeps sessions and
  tests reproducible.
- `?turbo=N` solo tick accelerator; `RTSSim.debugSpawn` +
  `core.DebugSpawnUnits` for load tests; a small `window.__rts`
  automation surface.
- Playwright E2E suite (`make test-e2e`, 4 specs): main menu → first
  harvest; turbo build/train/age-up; second browser context joins a
  hosted room with hash sync and no desync; 500-unit frame-rate
  regression guard. CI installs the Playwright browser and runs the
  whole matrix on three OSes; `NO_E2E=1` skips.

### Changed

- `session.start()` accepts an optional difficulty; `make test` now
  includes the E2E phase.

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
