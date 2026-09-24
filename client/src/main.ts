import * as THREE from 'three';

import {
  SimWorld,
  seedFromLocation,
  type BuildingDef,
  type BuildingRec,
  type NodeRec,
  type RelicRec,
  type UnitDef,
  type UnitRec,
} from './sim/bridge';
import { loadSim } from './sim/loader';
import {
  OrderKind,
  attackPayload,
  buildPayload,
  eraPayload,
  gatherPayload,
  moveGroupPayload,
  patrolPayload,
  repairPayload,
  researchPayload,
  TargetKind,
  stopPayload,
  trainPayload,
} from './sim/ordercodec';
import { NetSession } from './net/session';
import { menuFlow } from './ui/menu';
import { FixedStepLoop } from './engine/loop';
import { Interpolated } from './engine/interpolate';
import { ControlGroups } from './engine/groups';
import { FogView } from './render/fogView';
import { GroundChunks } from './render/terrainChunks';
import { Minimap } from './render/minimap';
import type { RenderUnit } from './render/actors';
import { Selection, normalizeRect, selectInRect, type ScreenPoint } from './engine/selection';
import { createScene, frameShadow } from './render/scene';
import { Daylight } from './render/daylight';
import { FireLights } from './render/lights';
import { buildMapObjects } from './render/objects';
import { buildNodeObjects } from './render/nodes';
import { buildActors } from './render/actors';
import { buildBuildings } from './render/buildings';
import { buildFX } from './render/fx';
import { buildingModel, unitModel } from './render/models';
import { heightSampler } from './render/height';
import { RtsCamera } from './render/camera';
import { HEIGHT_SCALE, PALETTE, PLAYER_COLORS } from './render/palette';
import { terrainName } from './render/terrainLabels';
import { t, tn } from './i18n';
import { createAudio } from './audio';
import type { Mood } from './audio/composition';
import { loadVolumes } from './audio/util';

let LOCAL_PLAYER = 0;
const NEUTRAL_OWNER = 255;

// Fauna body colors (keyed by unit def id) for actor insignia/HP tint.
const ANIMAL_TINTS: Record<number, number> = {
  5: PALETTE.deer,
  6: PALETTE.boar,
  9: PALETTE.rabbit,
  10: PALETTE.sheep,
  11: PALETTE.bear,
  12: PALETTE.wolf,
  13: PALETTE.blackWolf,
};
// unit and building entity ids collide (one counter per sim store), so
// building ids are offset inside the client-side selection
const BKEY = 1 << 30;
const KIND_NAMES: Record<number, string> = {
  1: 'node.wood',
  2: 'node.stone',
  3: 'node.food',
  4: 'node.gold',
  5: 'node.oil',
};

// Building ids mirrored from data/buildings.json.
const BLD_SAWMILL = 9;

// Spark colors for harvest pops, indexed by node kind.
const RESOURCE_NODE_COLORS: Record<number, number> = {
  1: PALETTE.leaf,
  2: PALETTE.stone,
  3: PALETTE.berry,
  4: PALETTE.gold,
  5: 0x333344,
};

const statusEl = document.querySelector<HTMLDivElement>('#status')!;
const statsEl = document.querySelector<HTMLDivElement>('#stats')!;
const tickEl = document.querySelector<HTMLSpanElement>('#tick')!;
const hashEl = document.querySelector<HTMLSpanElement>('#hash')!;
const mapEl = document.querySelector<HTMLSpanElement>('#map')!;
const fpsEl = document.querySelector<HTMLSpanElement>('#fps')!;
const selectedEl = document.querySelector<HTMLSpanElement>('#selected')!;
const hoverEl = document.querySelector<HTMLDivElement>('#hover')!;
const selRectEl = document.querySelector<HTMLDivElement>('#selrect')!;
const resbarEl = document.querySelector<HTMLDivElement>('#resbar')!;
const panelEl = document.querySelector<HTMLDivElement>('#panel')!;
const resEls = {
  wood: document.querySelector<HTMLSpanElement>('#res-wood')!,
  stone: document.querySelector<HTMLSpanElement>('#res-stone')!,
  food: document.querySelector<HTMLSpanElement>('#res-food')!,
  gold: document.querySelector<HTMLSpanElement>('#res-gold')!,
  pop: document.querySelector<HTMLSpanElement>('#res-pop')!,
  era: document.querySelector<HTMLSpanElement>('#res-era')!,
  relics: document.querySelector<HTMLSpanElement>('#res-relics')!,
};
const victoryEl = document.querySelector<HTMLDivElement>('#victory')!;
const canvas = document.querySelector<HTMLCanvasElement>('#gl')!;
const eraPipEls = Array.from(document.querySelectorAll<HTMLSpanElement>('#erapips i'));

// resource chip tooltips come from the locale table, never hard-coded
const CHIP_KEYS: Record<string, string> = {
  'chip-wood': 'res.wood',
  'chip-stone': 'res.stone',
  'chip-food': 'res.food',
  'chip-gold': 'res.gold',
  'chip-pop': 'res.pop',
  'chip-relics': 'res.relics',
};

statusEl.textContent = t('hud.status.loading');

for (const [id, key] of Object.entries(CHIP_KEYS)) {
  const el = document.getElementById(id);
  if (el) el.title = t(key as Parameters<typeof t>[0]);
}

const netEl = document.querySelector<HTMLDivElement>('#netstatus')!;
let session: NetSession | null = null;

function netStatus(msg: string): void {
  netEl.textContent = msg;
  netEl.hidden = !msg;
  netEl.classList.toggle('err', msg.startsWith('DESYNC') || msg.startsWith('error'));
}

try {
  const sim = await loadSim();
  let params = new URLSearchParams(window.location.search);
  if (!params.get('ws') && params.get('menu') !== 'off') {
    const defWs = `ws://${window.location.hostname}:8787`;
    const res = await menuFlow(defWs);
    window.history.replaceState({}, '', `?${res.params.toString()}`);
    params = res.params;
  }
  const wsUrl = params.get('ws');
  let seed: string;
  let playerCount = 0;
  if (wsUrl) {
    session = new NetSession(wsUrl, {
      onStatus: netStatus,
      onLobby: (code, players) => netStatus(`room ${code}: ${players.map((p) => p.name).join(', ')}`),
    });
    if (params.get('create') !== null) {
      session.create(params.get('name') || 'Player', Number(params.get('bots') ?? 0));
      const diff = params.get('difficulty') || undefined;
      if (params.get('autostart') !== null) session.start(diff);
      const after = Number(params.get('startafter') ?? '');
      if (after > 0) setTimeout(() => session?.start(diff), after);
    } else {
      session.join(
        params.get('code') || sessionStorage.getItem('rts.code') || '',
        params.get('name') || 'Player',
        sessionStorage.getItem('rts.token') || '',
      );
    }
    netStatus('connecting...');
    // available before boot completes: hosting an E2E match needs a
    // way to press "start" while the page waits for the match itself
    (window as unknown as Record<string, unknown>).__rts = {
      start: (diff?: string) => session?.start(diff),
      isOnline: () => true,
    };
    const boot = await session.booted();
    seed = boot.seed;
    LOCAL_PLAYER = boot.player;
    playerCount = boot.playerCount;
  } else {
    seed = seedFromLocation(window.location.search).toString(16);
    const soloBots = Number(params.get('bots') ?? 0);
    if (soloBots > 0) playerCount = 1 + soloBots;
  }
  const world = SimWorld.create(sim, seed, playerCount || undefined);
  if (!session) {
    const soloBots = Number(params.get('bots') ?? 0);
    const diff = params.get('difficulty') || 'standard';
    for (let slot = 1; slot < 1 + soloBots; slot++) world.brainNew(slot, diff);
  }
  // minimal automation surface for E2E tests (and console tinkering)
  const pre = ((window as unknown as Record<string, unknown>).__rts ??
    {}) as Record<string, unknown>;
  Object.assign(pre, {
    start: (diff?: string) => session?.start(diff),
    isOnline: () => session !== null,
  });
  Object.assign(pre, {
    world,
    issue,
    spawn: (owner: number, def: number, count: number) => world.debugSpawn(owner, def, count),
    local: () => LOCAL_PLAYER,
    tick: () => world.tick(),
    hash: () => world.hashHex(),
    victory: () => world.victory(),
    units: () => world.units(),
    nodes: () => world.nodes(),
    economy: () => world.economy(),
    kinds: OrderKind,
    payload: {
      gather: gatherPayload,
      build: buildPayload,
      train: trainPayload,
      era: eraPayload,
      attack: attackPayload,
      repair: repairPayload,
    },
  });
  (window as unknown as Record<string, unknown>).__rts = pre;
  if (session) {
    const boot = session.bootData!;
    for (let tt = 0; tt < boot.tick; tt++) {
      world.stepWith(boot.orders.get(tt) ?? []);
    }
    session.onTick = (tick, orders, hash) => {
      world.stepWith(orders);
      const local = world.hashHex();
      if (hash && local !== hash) {
        netStatus(`DESYNC at tick ${tick}: local ${local} server ${hash}`);
      }
      session!.confirm(tick, local);
    };
  }
  statusEl.hidden = true;
  statsEl.hidden = false;
  resbarEl.style.display = 'flex';
  console.info(t('hud.status.ready', { version: sim.version(), seed: world.seed() }));

  // audio is synthesized live; browsers only let it start on a user gesture
  const audio = createAudio(world.seed(), loadVolumes());
  let audioStarted = false;
  const unlockAudio = (): void => {
    if (audioStarted) return;
    audioStarted = true;
    window.removeEventListener('pointerdown', unlockAudio);
    window.removeEventListener('keydown', unlockAudio);
    void audio.start();
  };
  window.addEventListener('pointerdown', unlockAudio);
  window.addEventListener('keydown', unlockAudio);

  const size = world.mapSize();
  const tiles = world.mapTiles();
  const biomes = world.mapBiomes();
  const heights = world.vertexHeights();
  console.info(`map ${size}x${size} hash=${world.mapHashHex()}`);

  const bundle = createScene(canvas, size);
  const { renderer, scene, camera } = bundle;
  const daylight = new Daylight(
    bundle.sun,
    bundle.hemi,
    scene.fog as THREE.Fog,
    bundle.sky,
    renderer,
  );
  const blockedTiles = new Set<number>(world.nodes().map((n) => n.tileY * size + n.tileX));
  const ground = new GroundChunks(size, tiles, heights, blockedTiles, biomes);
  scene.add(ground.group);
  scene.add(buildMapObjects(size, heights, world.spawns()).group);

  let nodes = world.nodes();
  const nodesObj = buildNodeObjects(nodes, size, heights);
  scene.add(nodesObj.group);
  const actors = buildActors(size, heights, camera);
  scene.add(actors.group);
  const buildingsObj = buildBuildings(size, heights);
  scene.add(buildingsObj.group);
  const sampleGround = heightSampler(size, heights);
  const fx = buildFX(sampleGround);
  scene.add(fx.group);

  const defs = world.defs();
  const defById = new Map<number, BuildingDef>(defs.map((d) => [d.id, d]));
  const udefs = world.unitDefs();
  const udefById = new Map<number, UnitDef>(udefs.map((d) => [d.id, d]));
  const eraDefs = world.eras();
  const techDefs = world.techDefs();
  const biomeDefs = world.biomeDefs();
  const treeDefs = world.treeDefs();
  const treeById = new Map(treeDefs.map((d) => [d.id, d]));
  const techById = new Map(techDefs.map((d) => [d.id, d]));
  const campfireDefId = defs.find((d) => d.key === 'campfire')?.id ?? -1;
  const fireLights = new FireLights(scene);

  // Model heights (world units) drive body-aware picking: clicking a
  // roof or a head selects, not just the ground tile under the feet.
  const unitHeights = new Map<number, number>();
  for (const ud of udefs) {
    const g = unitModel(ud.id).body;
    g.computeBoundingBox();
    unitHeights.set(ud.id, g.boundingBox ? Math.max(g.boundingBox.max.y, 0.8) : 1.2);
  }
  const buildingHeights = new Map<number, number>();
  for (const d of defs) {
    const g = buildingModel(d.key).structure;
    g.computeBoundingBox();
    buildingHeights.set(d.id, g.boundingBox ? Math.max(g.boundingBox.max.y, 1) : 1.4);
  }
  const nodeByTile = new Map<number, NodeRec>();
  const nodeById = new Map<number, NodeRec>();
  const refreshNodeIndex = (): void => {
    nodeByTile.clear();
    nodeById.clear();
    for (const n of nodes) {
      nodeByTile.set(n.tileY * size + n.tileX, n);
      nodeById.set(n.id, n);
    }
  };
  refreshNodeIndex();

  const fogView = new FogView(size, world.fogBlob(LOCAL_PLAYER));
  ground.setFog(fogView);
  nodesObj.setFog(fogView);

  const minimapCanvas = document.querySelector<HTMLCanvasElement>('#minimap')!;
  minimapCanvas.width = 168;
  minimapCanvas.height = 168;
  const minimap = new Minimap(minimapCanvas, size, tiles, biomes);
  minimap.onPick = (x, y) => rtsCam.focus(x + 0.5, y + 0.5);
  const minimapEl = document.querySelector<HTMLDivElement>('#minimapbox')!;
  minimapEl.style.display = 'block';

  const interp = new Interpolated();

  const rtsCam = new RtsCamera(camera, size, sampleGround);
  const home = world.spawns()[LOCAL_PLAYER] ?? { x: size / 2, y: size / 2 };
  rtsCam.focus(home.x + 0.5, home.y + 0.5);
  rtsCam.zoom(0.5);
  mapEl.textContent = `${size}² ${world.mapHashHex().slice(0, 8)}`;

  let units: UnitRec[] = [];
  let buildings: BuildingRec[] = [];
  let relics: RelicRec[] = world.relics();
  let ownedTechs = new Set<number>(world.playerTechs(LOCAL_PLAYER));
  const selection = new Selection();
  let placing: BuildingDef | null = null;

  const groundY = (tx: number, ty: number): number =>
    heights[ty * (size + 1) + tx] * HEIGHT_SCALE;

  // placement ghost -------------------------------------------------------
  // a translucent silhouette of the actual building over a footprint tile
  // overlay that turns red where placement is impossible
  const ghostGroup = new THREE.Group();
  ghostGroup.visible = false;
  ghostGroup.matrixAutoUpdate = true;
  const footGeo = new THREE.PlaneGeometry(1, 1);
  footGeo.rotateX(-Math.PI / 2);
  const footMat = new THREE.MeshBasicMaterial({
    color: PALETTE.leaf,
    transparent: true,
    opacity: 0.32,
    depthWrite: false,
  });
  const foot = new THREE.Mesh(footGeo, footMat);
  foot.renderOrder = 2;
  ghostGroup.add(foot);
  const ghostMat = new THREE.MeshLambertMaterial({
    color: 0xffffff,
    transparent: true,
    opacity: 0.65,
    vertexColors: true,
    depthWrite: false,
  });
  let ghostSilhouette: THREE.Mesh | null = null;
  scene.add(ghostGroup);

  // placementBlocked reports whether a def.size square at tx,ty overlaps a
  // node, an existing building or the map border.
  function placementBlocked(tx: number, ty: number, s: number): boolean {
    if (tx < 0 || ty < 0 || tx + s > size || ty + s > size) return true;
    for (let dy = 0; dy < s; dy++) {
      for (let dx = 0; dx < s; dx++) {
        if (blockedTiles.has((ty + dy) * size + (tx + dx))) return true;
      }
    }
    for (const b of buildings) {
      const d = defById.get(b.def);
      const bs = d ? d.size : 1;
      if (tx < b.tileX + bs && b.tileX < tx + s && ty < b.tileY + bs && b.tileY < ty + s) return true;
    }
    return false;
  }

  // --- picking ----------------------------------------------------------
  const raycaster = new THREE.Raycaster();
  const proj = new THREE.Vector3();

  function tileAt(clientX: number, clientY: number): number | null {
    const ndc = new THREE.Vector2(
      (clientX / window.innerWidth) * 2 - 1,
      -(clientY / window.innerHeight) * 2 + 1,
    );
    raycaster.setFromCamera(ndc, camera);
    const hits = raycaster.intersectObjects(ground.meshes, false);
    if (hits.length === 0) return null;
    const tx = Math.floor(hits[0].point.x);
    const ty = Math.floor(hits[0].point.z);
    if (tx < 0 || ty < 0 || tx >= size || ty >= size) return null;
    return ty * size + tx;
  }

  function screenPos(world: THREE.Vector3): { sx: number; sy: number; onScreen: boolean } {
    proj.copy(world).project(camera);
    return {
      sx: (proj.x * 0.5 + 0.5) * window.innerWidth,
      sy: (-proj.y * 0.5 + 0.5) * window.innerHeight,
      onScreen: proj.z >= -1 && proj.z <= 1,
    };
  }

  function unitScreenPoints(): ScreenPoint[] {
    return units
      .filter((u) => u.owner === LOCAL_PLAYER)
      .map((u) => {
        const fy = groundY(Math.floor(u.x), Math.floor(u.y));
        const p = screenPos(new THREE.Vector3(u.x, fy, u.y));
        const top = screenPos(new THREE.Vector3(u.x, fy + (unitHeights.get(u.def) ?? 1.2), u.y));
        return { id: u.id, ...p, syTop: top.sy, onScreen: p.onScreen };
      });
  }

  // distSqToSeg is the squared distance from a point to a screen segment.
  function distSqToSeg(px: number, py: number, ax: number, ay: number, bx: number, by: number): number {
    const dx = bx - ax;
    const dy = by - ay;
    const l2 = dx * dx + dy * dy;
    const t = l2 === 0 ? 0 : Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / l2));
    const qx = ax + t * dx - px;
    const qy = ay + t * dy - py;
    return qx * qx + qy * qy;
  }

  function nearestUnit(clientX: number, clientY: number, radius: number): number | null {
    let best: number | null = null;
    let bestD = Infinity;
    for (const u of units) {
      if (u.owner !== LOCAL_PLAYER) continue;
      const fy = groundY(Math.floor(u.x), Math.floor(u.y));
      const p = screenPos(new THREE.Vector3(u.x, fy, u.y));
      if (!p.onScreen) continue;
      const top = screenPos(new THREE.Vector3(u.x, fy + (unitHeights.get(u.def) ?? 1.2), u.y));
      const w = Math.abs(screenPos(new THREE.Vector3(u.x + 0.5, fy, u.y)).sx - p.sx);
      const r = Math.max(radius, w * 3);
      const d = distSqToSeg(clientX, clientY, p.sx, p.sy, top.sx, top.sy);
      if (d <= r * r && d < bestD) {
        bestD = d;
        best = u.id;
      }
    }
    return best;
  }

  // nearestBuilding returns a selection key (raw building id + BKEY).
  function nearestBuilding(clientX: number, clientY: number, radius: number): number | null {
    let best: number | null = null;
    let bestD = Infinity;
    for (const b of buildings) {
      const def = defById.get(b.def);
      const half = ((def ? def.size : 1) - 1) / 2;
      const cx = b.tileX + 0.5 + half;
      const cz = b.tileY + 0.5 + half;
      const fy = groundY(b.tileX, b.tileY);
      const p = screenPos(new THREE.Vector3(cx, fy, cz));
      if (!p.onScreen) continue;
      const top = screenPos(new THREE.Vector3(cx, fy + (buildingHeights.get(b.def) ?? 1.4), cz));
      const w = Math.abs(screenPos(new THREE.Vector3(cx + 0.5, fy, cz)).sx - p.sx);
      const r = Math.max(radius, w * 2.6);
      const d = distSqToSeg(clientX, clientY, p.sx, p.sy, top.sx, top.sy);
      if (d <= r * r && d < bestD) {
        bestD = d;
        best = b.id + BKEY;
      }
    }
    return best;
  }

  function selectedVillagers(): number[] {
    const ids = selection.toArray();
    const owned = new Set(units.filter((u) => u.owner === LOCAL_PLAYER).map((u) => u.id));
    return ids.filter((id) => owned.has(id));
  }

  // gatherTargetAt resolves a right-click on ground to a harvestable
  // node: candidates in the 3x3 tile window are ranked by how close they
  // appear on screen to the cursor, so clicking a bush in front picks
  // that bush, not the forest tile behind it.
  function gatherTargetAt(tile: number, clientX: number, clientY: number): NodeRec | null {
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    let best: NodeRec | null = null;
    let bestD = Infinity;
    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        const n = nodeByTile.get((ty + dy) * size + (tx + dx));
        if (!n || n.kind === 5 || n.amount <= 0) continue;
        const p = screenPos(new THREE.Vector3(n.x, groundY(n.tileX, n.tileY) + 0.3, n.y));
        if (!p.onScreen) continue;
        const sx = p.sx - clientX;
        const sy = p.sy - clientY;
        const d = sx * sx + sy * sy;
        if (d < bestD) {
          bestD = d;
          best = n;
        }
      }
    }
    return best;
  }

  // --- orders -------------------------------------------------------------
  function issue(kind: number, payload: Uint8Array): void {
    if (session) session.sendOrder(kind, payload);
    else world.order(LOCAL_PLAYER, kind, payload);
  }

  function issueMove(tile: number): void {
    const ids = selectedVillagers();
    if (ids.length === 0) return;
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    issue(OrderKind.MoveGroup, moveGroupPayload(ids, tx * 16 + 8, ty * 16 + 8));
  }

  function issueGather(node: NodeRec): void {
    for (const id of selectedVillagers()) {
      issue(OrderKind.Gather, gatherPayload(id, node.id));
    }
  }

  function issueBuild(tile: number): void {
    if (!placing) return;
    const ids = selectedVillagers();
    if (ids.length === 0) return;
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    issue(OrderKind.Build, buildPayload(ids[0], placing.id, tx, ty));
    audio.place();
    setPlacing(null);
  }

  function issueTrain(buildingId: number, unitDef: number): void {
    issue(OrderKind.Train, trainPayload(buildingId, unitDef));
  }

  function issueAgeUp(era: number): void {
    issue(OrderKind.AgeUp, eraPayload(era));
  }

  function issueResearch(tech: number): void {
    issue(OrderKind.Research, researchPayload(tech));
  }

  function issueStop(): void {
    const ids = selection.toArray().filter((id) => id < BKEY);
    if (ids.length === 0) return;
    issue(OrderKind.Stop, stopPayload(ids));
  }

  // selectedUnits returns the live local-player units in the selection.
  function selectedUnits(): UnitRec[] {
    const ids = new Set(selection.toArray());
    return units.filter((u) => u.owner === LOCAL_PLAYER && ids.has(u.id));
  }

  function issueAttack(target: UnitRec): void {
    const ids = selectedUnits().map((u) => u.id);
    if (ids.length === 0) return;
    issue(OrderKind.Attack, attackPayload(ids, target.id, TargetKind.Unit));
  }

  function issueAttackBuilding(targetId: number): void {
    const ids = selectedUnits().map((u) => u.id);
    if (ids.length === 0) return;
    issue(OrderKind.Attack, attackPayload(ids, targetId, TargetKind.Building));
  }

  function issueRepair(targetId: number): void {
    const ids = selectedUnits().map((u) => u.id);
    if (ids.length === 0) return;
    issue(OrderKind.Repair, repairPayload(ids, targetId));
  }

  function issuePatrol(tile: number): void {
    const ids = selectedUnits().map((u) => u.id);
    if (ids.length === 0) return;
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    issue(OrderKind.Patrol, patrolPayload(ids, tx * 16 + 8, ty * 16 + 8));
  }

  // enemyUnitAt finds a visible hostile (players or wildlife) near a point.
  function enemyUnitAt(clientX: number, clientY: number, radius: number): UnitRec | null {
    let best: UnitRec | null = null;
    let bestD = Infinity;
    for (const u of units) {
      if (u.owner === LOCAL_PLAYER) continue;
      if (!fogView.visible(Math.floor(u.x), Math.floor(u.y))) continue;
      const fy = groundY(Math.floor(u.x), Math.floor(u.y));
      const p = screenPos(new THREE.Vector3(u.x, fy, u.y));
      if (!p.onScreen) continue;
      const top = screenPos(new THREE.Vector3(u.x, fy + (unitHeights.get(u.def) ?? 1.2), u.y));
      const w = Math.abs(screenPos(new THREE.Vector3(u.x + 0.5, fy, u.y)).sx - p.sx);
      const r = Math.max(radius, w * 3);
      const d = distSqToSeg(clientX, clientY, p.sx, p.sy, top.sx, top.sy);
      if (d <= r * r && d < bestD) {
        bestD = d;
        best = u;
      }
    }
    return best;
  }

  // --- panel --------------------------------------------------------------
  type CostKind = 'wood' | 'stone' | 'food' | 'gold';

  function canAfford(def: BuildingDef): boolean {
    const p = economy[LOCAL_PLAYER];
    return p !== undefined && p.res[1] >= def.costWood && p.res[2] >= def.costStone;
  }

  function canAffordUnit(def: UnitDef): boolean {
    const p = economy[LOCAL_PLAYER];
    return (
      p !== undefined &&
      p.res[3] >= def.costFood &&
      p.res[1] >= def.costWood &&
      p.res[4] >= def.costGold &&
      p.pop + def.popCost <= p.popCap
    );
  }

  function researchAffordable(td: { costFood: number; costWood: number; costGold: number }): boolean {
    const p = economy[LOCAL_PLAYER];
    return (
      p !== undefined &&
      p.res[3] >= td.costFood &&
      p.res[1] >= td.costWood &&
      p.res[4] >= td.costGold
    );
  }

  // The panel must not churn its DOM every 10 ticks: a mousedown on a
  // button destroyed by a mid-click rebuild never fires a click. HTML is
  // only re-assigned when its structural signature changes; progress
  // bars and percentage labels update in place via data hooks.
  interface PanelAction {
    act: 'place' | 'stop' | 'train' | 'research' | 'ageup';
    def?: number;
    bid?: number;
    era?: number;
    title?: string;
  }
  interface PanelDynamic {
    id: string;
    pct?: number;
    label?: string;
  }
  let panelActs: PanelAction[] = [];
  let panelDyn: PanelDynamic[] = [];
  let panelSig = '';
  let panelWired = false;

  function esc(s: string): string {
    return s.replace(
      /[&<>"']/g,
      (c) => (({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string),
    );
  }

  function iconHtml(kind: CostKind): string {
    return `<svg class="cost-ico" width="12" height="12"><use href="#i-${kind}"/></svg>`;
  }

  function renderPanel(): void {
    panelActs = [];
    panelDyn = [];
    const villagers = selectedVillagers().length;
    const soldiers = selectedUnits().filter(
      (u) => (udefById.get(u.def)?.class ?? 0) >= 2 && u.def !== 1,
    ).length;
    const selBuildings = selection
      .toArray()
      .map((id) => buildings.find((b) => id >= BKEY && b.id === id - BKEY))
      .filter((b): b is BuildingRec => b !== undefined && b.owner === LOCAL_PLAYER);

    if (villagers === 0 && soldiers === 0 && selBuildings.length === 0) {
      panelEl.style.display = 'none';
      panelSig = '';
      return;
    }

    const pl = economy[LOCAL_PLAYER];
    const era = pl ? pl.era : 0;
    const hintTxt = placing ? t('panel.place') : '';
    const btn = (
      action: PanelAction,
      disabled: boolean,
      label: string,
      costs: [CostKind, number][] = [],
      cls = '',
      title = '',
    ): string => {
      const i = panelActs.push(action) - 1;
      let inner = esc(label);
      for (const [kind, n] of costs) if (n > 0) inner += iconHtml(kind) + String(n);
      const tAttr = title ? ` title="${esc(title)}"` : '';
      return `<button data-i="${i}"${cls ? ` class="${cls}"` : ''}${tAttr}${disabled ? ' disabled' : ''}>${inner}</button>`;
    };
    const dynBar = (id: string, pct: number, blue: boolean): string => {
      panelDyn.push({ id, pct });
      return `<div class="bar${blue ? ' blue' : ''}"><i data-w="${esc(id)}"></i></div>`;
    };
    const dynText = (id: string, label: string, cls = ''): string => {
      panelDyn.push({ id, label });
      return `<div${cls ? ` class="${cls}"` : ''} data-pct="${esc(id)}"></div>`;
    };

    let html = '';
    if (villagers > 0) {
      html += `<div class="title">${esc(t('panel.villagers', { count: villagers }))}</div><div class="row">`;
      for (const def of defs) {
        if (def.minEra > era) continue;
        html += btn(
          { act: 'place', def: def.id },
          !canAfford(def),
          tn(`building.${def.key}`),
          [['wood', def.costWood], ['stone', def.costStone]],
        );
      }
      html += `</div><div class="dim" data-hint="1">${esc(hintTxt)}</div>`;
    }

    if (soldiers > 0) {
      html += `<div class="title">${esc(t('panel.military', { count: soldiers }))}</div><div class="row one">`;
      html += btn({ act: 'stop' }, false, t('panel.stop'));
      html += `</div><div class="dim" data-hint="1">${esc(hintTxt)}</div>`;
    }

    for (const b of selBuildings) {
      const def = defById.get(b.def);
      if (!def) continue;
      if (b.progress > 0) {
        const pct = Math.round(100 * (1 - b.progress / def.buildTicks));
        html += dynText(`c${b.id}`, `${tn(`building.${def.key}`)} · ${t('panel.progress', { pct })}`, 'title');
        html += dynBar(`c${b.id}`, pct, false);
        continue;
      }
      html += `<div class="title">${esc(tn(`building.${def.key}`))}</div>`;
      const trainable = udefs.filter((d) => d.trainBy === b.def);
      if (trainable.length === 0) continue;

      if (b.trainCur !== 0) {
        const cur = udefById.get(b.trainCur);
        const total = cur && cur.trainTicks > 0 ? cur.trainTicks : 1;
        const pct = Math.round(100 * (1 - b.trainLeft / total));
        const curName = cur ? tn(`unit.${cur.key}`) : '?';
        html += dynText(`t${b.id}`, `${t('panel.training')} ${curName} · ${t('panel.trainProgress', { pct })}`);
        html += dynBar(`t${b.id}`, pct, true);
        const pending = Math.max(0, b.trainReq - (b.trainLeft > 0 ? 0 : 1));
        if (pending > 0) html += dynText(`q${b.id}`, `${t('panel.queue')} ${pending}`, 'dim');
      } else if (b.trainReq > 0) {
        html += dynText(`q${b.id}`, `${t('panel.queue')} ${b.trainReq}`, 'dim');
      }

      html += '<div class="row">';
      for (const u of trainable) {
        html += btn(
          { act: 'train', bid: b.id, def: u.id },
          !canAffordUnit(u),
          tn(`unit.${u.key}`),
          [['food', u.costFood], ['wood', u.costWood], ['gold', u.costGold]],
        );
      }
      html += '</div>';
    }

    // research buttons for every selected tech building
    for (const b of selBuildings) {
      if (b.progress > 0) continue;
      const techs = techDefs.filter((td) => td.building === b.def && !ownedTechs.has(td.id));
      if (techs.length === 0) continue;
      html += '<div class="row">';
      for (const td of techs) {
        const reqsOk = td.requires.every((r) => ownedTechs.has(r));
        html += btn(
          { act: 'research', def: td.id },
          !reqsOk ||
            td.era > era ||
            pl === undefined ||
            !researchAffordable(td) ||
            (pl.techCur !== 0 && pl.techCur !== td.id),
          tn(`tech.${td.key}`),
          [['food', td.costFood], ['wood', td.costWood], ['gold', td.costGold]],
        );
      }
      html += '</div>';
    }

    // current research progress
    if (pl && pl.techCur !== 0) {
      const td = techById.get(pl.techCur);
      if (td) {
        const pct = Math.round(100 * (1 - pl.techLeft / td.ticks));
        html += dynText('rs', `${tn(`tech.${td.key}`)} · ${t('panel.researching', { pct })}`, 'dim');
        html += dynBar('rs', pct, true);
      }
    }

    // age up button
    if (pl && era < 4) {
      const next = eraDefs[era + 1];
      if (pl.ageLeft > 0) {
        const pct = Math.round(100 * (1 - pl.ageLeft / next.ageTicks));
        html += dynText('age', `${t('panel.aging', { era: tn(`era.${next.key}`) })} · ${pct}%`);
        html += dynBar('age', pct, false);
      } else {
        const req = defById.get(next.reqBuilding);
        const hasReq = buildings.some(
          (b) => b.owner === LOCAL_PLAYER && b.def === next.reqBuilding && b.progress === 0,
        );
        const canAge = pl.res[3] >= next.costFood && pl.res[4] >= next.costGold;
        html += '<div class="row one">';
        html += btn(
          { act: 'ageup', era: era + 1 },
          !hasReq || !canAge,
          t('panel.ageup', { era: tn(`era.${next.key}`) }),
          [['food', next.costFood], ['gold', next.costGold]],
          'gold',
          req && !hasReq ? tn(`building.${req.key}`) : '',
        );
        html += '</div>';
      }
    }

    const sig = `${html}\u0000${JSON.stringify(panelActs)}`;
    if (sig !== panelSig) {
      panelSig = sig;
      panelEl.innerHTML = html;
      if (!panelWired) {
        panelWired = true;
        panelEl.addEventListener('click', (e) => {
          const el = (e.target as HTMLElement).closest('button');
          if (!el) return;
          const a = panelActs[Number(el.dataset.i)];
          if (!a) return;
          switch (a.act) {
            case 'place': {
              const d = defById.get(a.def as number);
              if (d) setPlacing(d);
              break;
            }
            case 'stop':
              issueStop();
              break;
            case 'train':
              issueTrain(a.bid as number, a.def as number);
              break;
            case 'research':
              issueResearch(a.def as number);
              break;
            case 'ageup':
              issueAgeUp(a.era as number);
              break;
          }
        });
      }
    }
    for (const d of panelDyn) {
      if (d.pct !== undefined) {
        const fill = panelEl.querySelector(`i[data-w="${d.id}"]`) as HTMLElement | null;
        if (fill) fill.style.width = `${Math.max(2, Math.min(100, d.pct))}%`;
      }
      if (d.label !== undefined) {
        const lab = panelEl.querySelector(`[data-pct="${d.id}"]`);
        if (lab) lab.textContent = d.label;
      }
    }
    panelEl.style.display = 'block';
  }

  function setPlacing(def: BuildingDef | null): void {
    placing = def;
    if (def) {
      if (ghostSilhouette) ghostGroup.remove(ghostSilhouette);
      ghostSilhouette = new THREE.Mesh(buildingModel(def.key).structure, ghostMat);
      ghostSilhouette.renderOrder = 3;
      ghostGroup.add(ghostSilhouette);
    }
    ghostGroup.visible = def !== null;
    renderPanel();
  }

  // --- pointer input --------------------------------------------------------
  let leftStart: { x: number; y: number } | null = null;
  let rightStart: { x: number; yaw: number } | null = null;
  let rightDownOnCanvas = false;
  let rightRotated = false;
  let lastTile: number | null = null;

  canvas.addEventListener('contextmenu', (e) => e.preventDefault());

  canvas.addEventListener('pointerdown', (e) => {
    canvas.setPointerCapture(e.pointerId);
    if (e.button === 0) {
      leftStart = { x: e.clientX, y: e.clientY };
    } else if (e.button === 2) {
      rightStart = { x: e.clientX, yaw: rtsCam.yaw };
      rightDownOnCanvas = true;
      rightRotated = false;
    }
  });

  window.addEventListener('pointermove', (e) => {
    if (rightStart) {
      const dx = e.clientX - rightStart.x;
      if (Math.abs(dx) > 3) rightRotated = true;
      rtsCam.rotate(dx * 0.005);
      rightStart.x = e.clientX;
    }
    if (leftStart) {
      const dx = e.clientX - leftStart.x;
      const dy = e.clientY - leftStart.y;
      if (dx * dx + dy * dy > 144) {
        const r = normalizeRect(leftStart.x, leftStart.y, e.clientX, e.clientY);
        selRectEl.style.display = 'block';
        selRectEl.style.left = `${r.x0}px`;
        selRectEl.style.top = `${r.y0}px`;
        selRectEl.style.width = `${r.x1 - r.x0}px`;
        selRectEl.style.height = `${r.y1 - r.y0}px`;
      }
      return;
    }
    updateHover(e.clientX, e.clientY);
  });

  window.addEventListener('pointerup', (e) => {
    if (e.button === 2) {
      if (rightStart) {
        rightStart = null;
      } else {
        rightDownOnCanvas = false;
      }
      return;
    }
    if (e.button !== 0 || !leftStart) return;
    // A click with a few pixels of jitter is still a click: rect-select
    // projects the unit's foot position, which a 4px micro-drag misses,
    // so the unit under the cursor silently failed to select.
    const dx = e.clientX - leftStart.x;
    const dy = e.clientY - leftStart.y;
    const dragged = dx * dx + dy * dy > 144;
    selRectEl.style.display = 'none';
    const tile = tileAt(e.clientX, e.clientY);

    if (placing) {
      if (tile !== null) issueBuild(tile);
      leftStart = null;
      return;
    }

    if (dragged) {
      const rect = normalizeRect(leftStart.x, leftStart.y, e.clientX, e.clientY);
      selection.replace(selectInRect(unitScreenPoints(), rect));
    } else {
      const unit = nearestUnit(e.clientX, e.clientY, 18);
      if (unit !== null) {
        selection.replace([unit]);
      } else {
        const b = nearestBuilding(e.clientX, e.clientY, 26);
        if (b !== null) {
          selection.replace([b]);
        } else {
          selection.clear();
        }
      }
    }
    applySelection();
    audio.click();
    leftStart = null;
  });

  window.addEventListener('pointerup', (e) => {
    if (e.button !== 2) return;
    const downOnCanvas = rightDownOnCanvas;
    const rotated = rightRotated;
    rightDownOnCanvas = false;
    rightRotated = false;
    if (!downOnCanvas) return; // right-click began on the HUD: no world order
    if (rotated && !placing) return; // right-drag rotates the camera, it is not an order
    if (placing) {
      setPlacing(null);
      return;
    }
    const tile = tileAt(e.clientX, e.clientY);
    if (tile === null) return;
    const playerCol = PLAYER_COLORS[LOCAL_PLAYER % PLAYER_COLORS.length];
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    const enemy = enemyUnitAt(e.clientX, e.clientY, 22);
    if (enemy) {
      issueAttack(enemy);
      audio.attackOrder();
      fx.ring(enemy.x, enemy.y, 0xe05050);
      return;
    }
    if (e.shiftKey && selectedUnits().length > 0) {
      issuePatrol(tile);
      audio.order();
      fx.ring(tx + 0.5, ty + 0.5, playerCol);
      return;
    }
    const bid = nearestBuilding(e.clientX, e.clientY, 26);
    if (bid !== null) {
      const b = buildings.find((x) => x.id === bid - BKEY);
      if (b && b.owner === LOCAL_PLAYER && b.progress > 0) {
        issueRepair(b.id);
        audio.order();
        fx.ring(b.tileX + 0.5, b.tileY + 0.5, playerCol);
        return;
      }
      if (b) {
        issueAttackBuilding(b.id);
        audio.attackOrder();
        fx.ring(b.tileX + 0.5, b.tileY + 0.5, 0xe05050);
        return;
      }
    }
    const node = gatherTargetAt(tile, e.clientX, e.clientY);
    if (node) {
      issueGather(node);
      audio.order();
      fx.ring(node.x, node.y, playerCol);
    } else {
      issueMove(tile);
      audio.order();
      fx.ring(tx + 0.5, ty + 0.5, playerCol);
    }
  });

  window.addEventListener('keydown', (e) => {
    if (e.code === 'Escape') setPlacing(null);
    handleGroupKeys(e);
  });

  // --- control groups ------------------------------------------------------
  const groups = new ControlGroups();

  function handleGroupKeys(e: KeyboardEvent): void {
    const m = /^Digit([1-9])$/.exec(e.code);
    if (!m) return;
    const slot = Number(m[1]);
    if (e.ctrlKey) {
      const ids = selection.toArray();
      if (e.shiftKey) groups.addTo(slot, ids);
      else groups.bind(slot, ids);
      return;
    }
    const aliveIds = groups.recall(slot, (id) =>
      units.some((u) => u.id === id && u.owner === LOCAL_PLAYER),
    );
    if (aliveIds.length > 0) {
      selection.replace(aliveIds);
      applySelection();
    }
  }

  function applySelection(): void {
    selectedEl.textContent = String(selection.size);
    renderPanel();
  }

  function updateHover(clientX: number, clientY: number): void {
    const tile = tileAt(clientX, clientY);
    lastTile = tile;
    if (tile === null) {
      hoverEl.style.visibility = 'hidden';
      return;
    }
    // units answer first so hovering your own or a visible hostile names it
    const own = nearestUnit(clientX, clientY, 15);
    const hovered = own !== null ? units.find((u) => u.id === own) : enemyUnitAt(clientX, clientY, 15);
    if (hovered) {
      const ud = udefById.get(hovered.def);
      hoverEl.textContent = ud ? `${tn(`unit.${ud.key}`)} · ${hovered.hp}/${ud.hp}` : `unit ${hovered.def}`;
      hoverEl.style.visibility = 'visible';
      return;
    }
    const node = nodeByTile.get(tile);
    if (node && node.kind !== 5) {
      const sp = node.tree ? treeById.get(node.tree) : undefined;
      const label = sp ? tn(`tree.${sp.key}`) : tn(KIND_NAMES[node.kind]);
      const gated =
        sp?.sawmill === true &&
        !buildings.some((b) => b.owner === LOCAL_PLAYER && b.def === BLD_SAWMILL && b.progress === 0);
      hoverEl.textContent = node.amount
        ? `${label} · ${node.amount}${gated ? ` · ${tn('tree.needSawmill')}` : ''}`
        : t('node.depleted');
    } else {
      const tx = tile % size;
      const ty = Math.floor(tile / size);
      const b = biomeDefs[biomes[tile]];
      const farm = b ? ` · ${tn(`biome.${b.key}`)} ×${b.farmMul / 1000}` : '';
      hoverEl.textContent = `${terrainName(tiles[tile])}${farm} · ${t('hud.height')} ${heights[ty * (size + 1) + tx]}`;
    }
    hoverEl.style.visibility = 'visible';
  }

  // --- keyboard + edge scroll --------------------------------------------
  const keys = new Set<string>();
  window.addEventListener('keydown', (e) => keys.add(e.code));
  window.addEventListener('keyup', (e) => keys.delete(e.code));
  let pointerPos = { x: -1, y: -1 };
  window.addEventListener('pointermove', (e) => {
    pointerPos = { x: e.clientX, y: e.clientY };
  });

  canvas.addEventListener(
    'wheel',
    (e) => {
      e.preventDefault();
      rtsCam.zoom(Math.exp(e.deltaY * 0.001));
    },
    { passive: false },
  );

  function driveCamera(): void {
    const panSpeed = rtsCam.dist * 0.012;
    let dx = 0;
    let dz = 0;
    if (keys.has('KeyW') || keys.has('ArrowUp')) dz += panSpeed;
    if (keys.has('KeyS') || keys.has('ArrowDown')) dz -= panSpeed;
    if (keys.has('KeyD') || keys.has('ArrowRight')) dx += panSpeed;
    if (keys.has('KeyA') || keys.has('ArrowLeft')) dx -= panSpeed;
    if (keys.has('KeyE')) rtsCam.rotate(0.02);
    if (keys.has('KeyQ')) rtsCam.rotate(-0.02);

    const edge = 12;
    if (pointerPos.x >= 0) {
      if (pointerPos.x < edge) dx -= panSpeed;
      if (pointerPos.x > window.innerWidth - edge) dx += panSpeed;
      if (pointerPos.y < edge) dz += panSpeed;
      if (pointerPos.y > window.innerHeight - edge) dz -= panSpeed;
    }
    if (dx !== 0 || dz !== 0) rtsCam.pan(dx, dz);
  }

  // --- HUD ----------------------------------------------------------------
  let economy = world.economy();

  // audio bookkeeping for cosmetic-only triggers
  const knownBuilt = new Set<number>(
    world
      .buildings()
      .filter((b) => b.owner === LOCAL_PLAYER && b.progress === 0)
      .map((b) => b.id),
  );
  let lastEra = economy[LOCAL_PLAYER]?.era ?? 0;
  let audioOver = false;

  function setVal(el: HTMLSpanElement, v: number | string): void {
    const s = String(v);
    if (el.textContent === s) return;
    el.textContent = s;
    el.classList.remove('bump');
    void el.offsetWidth;
    el.classList.add('bump');
    window.setTimeout(() => el.classList.remove('bump'), 700);
  }

  function renderHud(): void {
    const p = economy[LOCAL_PLAYER];
    if (!p) return;
    setVal(resEls.wood, p.res[1]);
    setVal(resEls.stone, p.res[2]);
    setVal(resEls.food, p.res[3]);
    setVal(resEls.gold, p.res[4]);
    resEls.pop.textContent = `${p.pop}/${p.popCap}`;
    const ed = eraDefs[p.era];
    resEls.era.textContent = ed ? tn(`era.${ed.key}`) : String(p.era);
    eraPipEls.forEach((pip, i) => pip.classList.toggle('on', i <= p.era));
    resEls.relics.textContent = String(relics.filter((r) => r.owner === LOCAL_PLAYER).length);
  }

  // resource floaters: small +N texts rising from harvest points, pooled
  const floaters: { el: HTMLDivElement; until: number }[] = [];
  function floater(x: number, y: number, text: string, color: number): void {
    let slot = floaters.find((f) => f.until <= performance.now());
    if (!slot) {
      if (floaters.length >= 24) return;
      const el = document.createElement('div');
      el.className = 'floater';
      document.body.appendChild(el);
      slot = { el, until: 0 };
      floaters.push(slot);
    }
    const el = slot.el;
    el.textContent = text;
    el.style.color = `#${color.toString(16).padStart(6, '0')}`;
    el.style.left = `${Math.round(x)}px`;
    el.style.top = `${Math.round(y)}px`;
    el.style.transition = 'none';
    el.style.transform = 'translate(-50%, 0)';
    el.style.opacity = '1';
    void el.offsetWidth;
    el.style.transition = 'transform 0.9s ease-out, opacity 0.9s ease-in';
    el.style.transform = 'translate(-50%, -38px)';
    el.style.opacity = '0';
    slot.until = performance.now() + 950;
  }

  // --- main loop ----------------------------------------------------------
  let smoothedMs = 16;
  let frameStart = performance.now();
  let lastLoggedTick = -1;
  let lastFogTick = -1;

  // last-seen snapshots for effect triggers (client-only bookkeeping)
  const lastVis = new Map<number, { hp: number; x: number; y: number; owner: number }>();
  const lastNodeAmt = new Map<number, number>();

  function commitSnapshot(): void {
    units = world.units();
    interp.commit(units.map((u) => [u.id, { x: u.x, y: u.y }] as [number, { x: number; y: number }]));
    // hit sparks and death puffs from hp deltas between snapshots
    const seen = new Set<number>();
    for (const u of units) {
      seen.add(u.id);
      const prev = lastVis.get(u.id);
      if (!prev) {
        lastVis.set(u.id, { hp: u.hp, x: u.x, y: u.y, owner: u.owner });
        continue;
      }
      if (u.hp < prev.hp) {
        const y = groundY(Math.floor(u.x), Math.floor(u.y));
        fx.burst(u.x, y, u.y, 0xffd27a, 2, 0.7);
        if (fogView.visible(Math.floor(u.x), Math.floor(u.y))) audio.hit();
        prev.hp = u.hp;
        prev.x = u.x;
        prev.y = u.y;
      } else {
        prev.hp = u.hp;
        prev.x = u.x;
        prev.y = u.y;
      }
    }
    for (const [id, u] of lastVis) {
      if (seen.has(id)) continue;
      const y = groundY(Math.floor(u.x), Math.floor(u.y));
      const col = u.owner === NEUTRAL_OWNER ? 0xa08a6a : PLAYER_COLORS[u.owner % PLAYER_COLORS.length];
      if (fogView.visible(Math.floor(u.x), Math.floor(u.y))) {
        fx.burst(u.x, y, u.y, col, 8);
        audio.death();
      }
      lastVis.delete(id);
    }
  }

  function resolveTarget(id: number): { x: number; y: number } | null {
    for (const u of units) {
      if (u.id === id) return { x: u.x, y: u.y };
    }
    for (const b of buildings) {
      if (b.id === id) {
        const d = defById.get(b.def);
        const s = d ? d.size : 1;
        return { x: b.tileX + s / 2, y: b.tileY + s / 2 };
      }
    }
    const n = nodeById.get(id);
    if (n) return { x: n.x, y: n.y };
    return null;
  }

  // ?turbo=N runs N sim ticks per loop iteration (solo only); used by
  // E2E to fast-forward long deterministic games in real time.
  const turbo = session ? 1 : Math.min(50, Math.max(1, Number(params.get('turbo') ?? 1)));
  const loop = new FixedStepLoop(
    () => {
      if (!session) {
        for (let i = 0; i < turbo; i++) world.step();
      }
      commitSnapshot();
    },
    () => {
      const now = performance.now();
      const fdt = Math.min(0.1, (now - frameStart) / 1000);
      smoothedMs = smoothedMs * 0.9 + (now - frameStart) * 0.1;
      frameStart = now;

      const alpha = loop.alpha();
      const renderUnits: RenderUnit[] = [];
      for (const u of units) {
        const mine = u.owner === LOCAL_PLAYER;
        if (!mine && !fogView.visible(Math.floor(u.x), Math.floor(u.y))) continue;
        const p = interp.get(u.id, alpha) ?? { x: u.x, y: u.y };
        const def = udefById.get(u.def);
        const color =
          u.owner === NEUTRAL_OWNER
            ? ANIMAL_TINTS[u.def] ?? PALETTE.deer
            : PLAYER_COLORS[u.owner % PLAYER_COLORS.length];
        const hpFrac = def && def.hp > 0 ? u.hp / def.hp : 1;
        renderUnits.push({
          id: u.id,
          owner: u.owner,
          x: p.x,
          y: p.y,
          carried: u.carried,
          carriedKind: u.carriedKind,
          def: u.def,
          cls: def?.class ?? 1,
          state: u.state,
          target: u.target,
          color,
          hpFrac,
        });
      }
      actors.setUnits(renderUnits, now, resolveTarget);
      buildings = world.buildings().filter(
        (b) => b.owner === LOCAL_PLAYER || fogView.visible(b.tileX, b.tileY),
      );
      buildingsObj.setBuildings(buildings, defById, now);
      // Refresh every frame so rings track interpolated movement.
      const selKeys = selection.toArray();
      actors.setSelection(selKeys.filter((id) => id < BKEY));
      buildingsObj.setSelection(selKeys.filter((id) => id >= BKEY).map((id) => id - BKEY));
      fx.update(fdt);

      const tick = world.tick();
      if (tick % 10 === 0) {
        nodes = world.nodes();
        nodesObj.update(nodes);
        refreshNodeIndex();
        for (const n of nodes) {
          const prev = lastNodeAmt.get(n.id);
          if (prev !== undefined && n.amount < prev - 0) {
            if (fogView.visible(n.tileX, n.tileY)) {
              const kindCol = RESOURCE_NODE_COLORS[n.kind] ?? PALETTE.stone;
              fx.burst(n.x, groundY(n.tileX, n.tileY) + 0.3, n.y, kindCol, 2, 0.5);
              const fp = screenPos(new THREE.Vector3(n.x, groundY(n.tileX, n.tileY) + 0.7, n.y));
              if (fp.onScreen) floater(fp.sx, fp.sy, `+${prev - n.amount}`, kindCol);
            }
          }
          lastNodeAmt.set(n.id, n.amount);
        }
        economy = world.economy();
        relics = world.relics();
        ownedTechs = new Set(world.playerTechs(LOCAL_PLAYER));
        renderHud();
        renderPanel();

        // procedural music inputs: mood from visible battle state, timbre
        // from the biome under the camera
        const cx = Math.floor(rtsCam.target.x);
        const cz = Math.floor(rtsCam.target.z);
        const bt = biomeDefs[biomes[Math.max(0, Math.min(size * size - 1, cz * size + cx))]];
        audio.setBiome(bt ? bt.key : 'temperate');
        let mood: Mood = 'calm';
        // state 5 == sim UnitAttack
        if (units.some((u) => u.state === 5 && u.owner !== NEUTRAL_OWNER)) mood = 'combat';
        else if (
          units.some(
            (u) =>
              u.owner !== LOCAL_PLAYER &&
              u.owner !== NEUTRAL_OWNER &&
              Math.abs(u.x - rtsCam.target.x) < 40 &&
              Math.abs(u.y - rtsCam.target.z) < 40 &&
              fogView.visible(Math.floor(u.x), Math.floor(u.y)),
          )
        )
          mood = 'tension';
        audio.setMood(mood);
        for (const b of world.buildings()) {
          if (b.owner === LOCAL_PLAYER && b.progress === 0 && !knownBuilt.has(b.id)) {
            knownBuilt.add(b.id);
            audio.buildDone();
          }
        }
        const eraNow = economy[LOCAL_PLAYER]?.era ?? 0;
        if (eraNow > lastEra) audio.eraUp();
        lastEra = eraNow;

        const vic = world.victory();
        if (vic.winner !== 255) {
          if (!audioOver) {
            audioOver = true;
            audio.endMatch(vic.winner === LOCAL_PLAYER);
          }
          const kinds = { 1: 'victory.1', 2: 'victory.2', 3: 'victory.3' } as const;
          victoryEl.style.display = 'block';
          victoryEl.textContent = t('victory.text', {
            player: vic.winner + 1,
            kind: t(kinds[vic.kind as 1 | 2 | 3] ?? 'victory.1'),
          });
        }
      }
      if (tick !== lastFogTick) {
        lastFogTick = tick;
        fogView.update(world.fogBlob(LOCAL_PLAYER));
        ground.setFog(fogView);
        nodesObj.setFog(fogView);
      }

      if (placing && lastTile !== null) {
        const tx = lastTile % size;
        const ty = Math.floor(lastTile / size);
        const s = placing.size;
        const ok = canAfford(placing) && !placementBlocked(tx, ty, s);
        ghostGroup.visible = true;
        ghostGroup.position.set(tx + s / 2, groundY(tx, ty), ty + s / 2);
        foot.scale.set(s, 1, s);
        const col = ok ? PALETTE.leaf : 0xd94f4f;
        footMat.color.setHex(col);
        ghostMat.color.setHex(ok ? 0xffffff : 0xff8a7a);
      }

      driveCamera();
      rtsCam.update();
      ground.cull(camera);
      ground.setTime(now / 1000);
      daylight.update(tick);
      frameShadow(
        bundle,
        rtsCam.target.x,
        rtsCam.target.z,
        THREE.MathUtils.clamp(rtsCam.dist * 0.9, 24, 90),
        daylight.dir,
      );
      fireLights.update(
        buildings
          .filter((b) => b.def === campfireDefId && b.progress === 0)
          .map((b) => {
            const d = defById.get(b.def);
            const half = (d ? d.size : 1) / 2;
            return {
              id: b.id,
              x: b.tileX + half,
              y: groundY(b.tileX, b.tileY) + 0.14,
              z: b.tileY + half,
            };
          }),
        rtsCam.target.x,
        rtsCam.target.z,
        daylight.night,
        now / 1000,
      );
      minimap.draw(fogView, renderUnits, {
        x: rtsCam.target.x,
        z: rtsCam.target.z,
        view: Math.min(size, rtsCam.dist * 1.2),
      }, relics, nodes, buildings.map((b) => {
        const d = defById.get(b.def);
        const half = (d ? d.size : 1) / 2;
        return { x: b.tileX + half, y: b.tileY + half, owner: b.owner };
      }));
      renderer.render(scene, camera);

      tickEl.textContent = String(tick);
      hashEl.textContent = world.hashHex();
      fpsEl.textContent = String(Math.round(1000 / smoothedMs));

      if (tick % 20 === 0 && tick !== lastLoggedTick) {
        lastLoggedTick = tick;
        console.info(`tick=${tick} hash=${world.hashHex()}`);
      }
    },
    () => performance.now(),
  );
  loop.start();
} catch (err) {
  statusEl.textContent = t('hud.status.failed');
  console.error(err);
}
