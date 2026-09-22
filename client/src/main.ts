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
  researchPayload,
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
import { createScene } from './render/scene';
import { buildMapObjects } from './render/objects';
import { buildNodeObjects } from './render/nodes';
import { buildActors } from './render/actors';
import { heightSampler } from './render/height';
import { RtsCamera } from './render/camera';
import { HEIGHT_SCALE, PALETTE, PLAYER_COLORS } from './render/palette';
import { terrainName } from './render/terrainLabels';
import { t, tn } from './i18n';

let LOCAL_PLAYER = 0;
const NEUTRAL_OWNER = 255;
// Moving states get the walk bob: move, attack (chase), patrol, wander, flee.
const MOVING_STATES = new Set([1, 5, 6, 7, 8]);
const KIND_NAMES: Record<number, string> = {
  1: 'node.wood',
  2: 'node.stone',
  3: 'node.food',
  4: 'node.gold',
  5: 'node.oil',
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

statusEl.textContent = t('hud.status.loading');

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

  const size = world.mapSize();
  const tiles = world.mapTiles();
  const heights = world.vertexHeights();
  console.info(`map ${size}x${size} hash=${world.mapHashHex()}`);

  const { renderer, scene, camera } = createScene(canvas, size);
  const ground = new GroundChunks(size, tiles, heights);
  scene.add(ground.group);
  scene.add(buildMapObjects(size, heights, world.spawns()).group);

  let nodes = world.nodes();
  const nodesObj = buildNodeObjects(nodes, size, heights);
  scene.add(nodesObj.group);
  const actors = buildActors(size, heights);
  scene.add(actors.group);

  const defs = world.defs();
  const defById = new Map<number, BuildingDef>(defs.map((d) => [d.id, d]));
  const udefs = world.unitDefs();
  const udefById = new Map<number, UnitDef>(udefs.map((d) => [d.id, d]));
  const eraDefs = world.eras();
  const techDefs = world.techDefs();
  const techById = new Map(techDefs.map((d) => [d.id, d]));
  const nodeByTile = new Map<number, NodeRec>();
  const refreshNodeIndex = (): void => {
    nodeByTile.clear();
    for (const n of nodes) nodeByTile.set(n.tileY * size + n.tileX, n);
  };
  refreshNodeIndex();

  const fogView = new FogView(size, world.fogBlob(LOCAL_PLAYER));
  ground.setFog(fogView);
  nodesObj.setFog(fogView);

  const minimapCanvas = document.querySelector<HTMLCanvasElement>('#minimap')!;
  minimapCanvas.width = 168;
  minimapCanvas.height = 168;
  const minimap = new Minimap(minimapCanvas, size, tiles);
  minimap.onPick = (x, y) => rtsCam.focus(x + 0.5, y + 0.5);
  const minimapEl = document.querySelector<HTMLDivElement>('#minimapbox')!;
  minimapEl.style.display = 'block';

  const interp = new Interpolated();

  const rtsCam = new RtsCamera(camera, size, heightSampler(size, heights));
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
  let affordable = false;

  const groundY = (tx: number, ty: number): number =>
    heights[ty * (size + 1) + tx] * HEIGHT_SCALE;

  // placement ghost -------------------------------------------------------
  const ghostGeo = new THREE.BoxGeometry(1, 1, 1);
  ghostGeo.translate(0.5, 0, 0.5);
  const ghostMat = new THREE.MeshBasicMaterial({
    color: PALETTE.leaf,
    transparent: true,
    opacity: 0.45,
  });
  const ghost = new THREE.Mesh(ghostGeo, ghostMat);
  ghost.visible = false;
  scene.add(ghost);

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
        const p = screenPos(
          new THREE.Vector3(u.x, groundY(Math.floor(u.x), Math.floor(u.y)), u.y),
        );
        return { id: u.id, ...p };
      });
  }

  function nearestUnit(clientX: number, clientY: number, radius: number): number | null {
    let best: number | null = null;
    let bestD2 = radius * radius;
    for (const u of unitScreenPoints()) {
      if (!u.onScreen) continue;
      const d2 = (u.sx - clientX) ** 2 + (u.sy - clientY) ** 2;
      if (d2 < bestD2) {
        bestD2 = d2;
        best = u.id;
      }
    }
    return best;
  }

  function nearestBuilding(clientX: number, clientY: number, radius: number): number | null {
    let best: number | null = null;
    let bestD2 = radius * radius;
    for (const b of buildings) {
      const def = defById.get(b.def);
      const half = ((def ? def.size : 1) - 1) / 2;
      const p = screenPos(
        new THREE.Vector3(
          b.tileX + 0.5 + half,
          groundY(b.tileX, b.tileY) + 0.5,
          b.tileY + 0.5 + half,
        ),
      );
      if (!p.onScreen) continue;
      const d2 = (p.sx - clientX) ** 2 + (p.sy - clientY) ** 2;
      if (d2 < bestD2) {
        bestD2 = d2;
        best = b.id;
      }
    }
    return best;
  }

  function selectedVillagers(): number[] {
    const ids = selection.toArray();
    const owned = new Set(units.filter((u) => u.owner === LOCAL_PLAYER).map((u) => u.id));
    return ids.filter((id) => owned.has(id));
  }

  function gatherTargetAt(tile: number): NodeRec | null {
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        const n = nodeByTile.get((ty + dy) * size + (tx + dx));
        if (n && n.kind !== 5 && n.amount > 0) return n;
      }
    }
    return null;
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
    const ids = selection.toArray();
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
    issue(OrderKind.Attack, attackPayload(ids, target.id));
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
    let bestD2 = radius * radius;
    for (const u of units) {
      if (u.owner === LOCAL_PLAYER) continue;
      if (!fogView.visible(Math.floor(u.x), Math.floor(u.y))) continue;
      const p = screenPos(
        new THREE.Vector3(u.x, groundY(Math.floor(u.x), Math.floor(u.y)), u.y),
      );
      if (!p.onScreen) continue;
      const d2 = (p.sx - clientX) ** 2 + (p.sy - clientY) ** 2;
      if (d2 < bestD2) {
        bestD2 = d2;
        best = u;
      }
    }
    return best;
  }

  // --- panel --------------------------------------------------------------
  function costLabel(def: BuildingDef): string {
    const parts: string[] = [];
    if (def.costWood > 0) parts.push(`${def.costWood}w`);
    if (def.costStone > 0) parts.push(`${def.costStone}s`);
    return parts.join(' ');
  }

  function canAfford(def: BuildingDef): boolean {
    const p = economy[LOCAL_PLAYER];
    return p !== undefined && p.res[1] >= def.costWood && p.res[2] >= def.costStone;
  }

  function unitCostLabel(def: UnitDef): string {
    const parts: string[] = [];
    if (def.costFood > 0) parts.push(`${def.costFood}f`);
    if (def.costWood > 0) parts.push(`${def.costWood}w`);
    if (def.costGold > 0) parts.push(`${def.costGold}g`);
    return parts.join(' ');
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

  function techCostLabel(td: { costFood: number; costWood: number; costGold: number }): string {
    const parts: string[] = [];
    if (td.costFood > 0) parts.push(`${td.costFood}f`);
    if (td.costWood > 0) parts.push(`${td.costWood}w`);
    if (td.costGold > 0) parts.push(`${td.costGold}g`);
    return parts.join(' ');
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

  function renderPanel(): void {
    const villagers = selectedVillagers().length;
    const soldiers = selectedUnits().filter(
      (u) => (udefById.get(u.def)?.class ?? 0) >= 2 && u.def !== 1,
    ).length;
    const selBuildings = selection
      .toArray()
      .map((id) => buildings.find((b) => b.id === id))
      .filter((b): b is BuildingRec => b !== undefined && b.owner === LOCAL_PLAYER);

    if (villagers === 0 && soldiers === 0 && selBuildings.length === 0) {
      panelEl.style.display = 'none';
      return;
    }
    panelEl.style.display = 'block';
    panelEl.textContent = '';

    const pl = economy[LOCAL_PLAYER];
    const era = pl ? pl.era : 0;

    if (villagers > 0) {
      const title = document.createElement('div');
      title.textContent = t('panel.villagers', { count: villagers });
      panelEl.appendChild(title);

      const row = document.createElement('div');
      row.className = 'row';
      for (const def of defs) {
        if (def.minEra > era) continue;
        const btn = document.createElement('button');
        btn.textContent = `${tn(`building.${def.key}`)} · ${costLabel(def)}`;
        btn.disabled = !canAfford(def);
        btn.onclick = () => setPlacing(def);
        row.appendChild(btn);
      }
      const hint = document.createElement('div');
      hint.className = 'dim';
      hint.dataset.hint = '1';
      panelEl.appendChild(row);
      panelEl.appendChild(hint);
    }

    if (soldiers > 0) {
      const title = document.createElement('div');
      title.textContent = t('panel.military', { count: soldiers });
      panelEl.appendChild(title);
      const row = document.createElement('div');
      row.className = 'row';
      const stop = document.createElement('button');
      stop.textContent = t('panel.stop');
      stop.onclick = () => issueStop();
      row.appendChild(stop);
      const hint = document.createElement('div');
      hint.className = 'dim';
      hint.dataset.hint = '1';
      panelEl.appendChild(row);
      panelEl.appendChild(hint);
    }

    for (const b of selBuildings) {
      const def = defById.get(b.def);
      if (!def) continue;
      const line = document.createElement('div');
      if (b.progress > 0) {
        const pct = Math.round(100 * (1 - b.progress / def.buildTicks));
        line.textContent = `${tn(`building.${def.key}`)} · ${t('panel.progress', { pct })}`;
        panelEl.appendChild(line);
        continue;
      }
      line.textContent = tn(`building.${def.key}`);
      panelEl.appendChild(line);

      const trainable = udefs.filter((d) => d.trainBy === b.def);
      if (trainable.length === 0) continue;

      if (b.trainCur !== 0) {
        const cur = udefById.get(b.trainCur);
        const total = cur && cur.trainTicks > 0 ? cur.trainTicks : 1;
        const pct = Math.round(100 * (1 - b.trainLeft / total));
        const curName = cur ? tn(`unit.${cur.key}`) : '?';
        const qline = document.createElement('div');
        qline.textContent = `${t('panel.training')} ${curName} · ${t('panel.trainProgress', { pct })}`;
        panelEl.appendChild(qline);
        const pending = Math.max(0, b.trainReq - (b.trainLeft > 0 ? 0 : 1));
        if (pending > 0) {
          const q = document.createElement('div');
          q.className = 'dim';
          q.textContent = `${t('panel.queue')} ${pending}`;
          panelEl.appendChild(q);
        }
      } else if (b.trainReq > 0) {
        const q = document.createElement('div');
        q.className = 'dim';
        q.textContent = `${t('panel.queue')} ${b.trainReq}`;
        panelEl.appendChild(q);
      }

      const row = document.createElement('div');
      row.className = 'row';
      for (const u of trainable) {
        const btn = document.createElement('button');
        btn.textContent = `${tn(`unit.${u.key}`)} · ${unitCostLabel(u)}`;
        btn.disabled = !canAffordUnit(u);
        btn.onclick = () => issueTrain(b.id, u.id);
        row.appendChild(btn);
      }
      panelEl.appendChild(row);
    }

    // research buttons for every selected tech building
    for (const b of selBuildings) {
      if (b.progress > 0) continue;
      const techs = techDefs.filter((td) => td.building === b.def && !ownedTechs.has(td.id));
      if (techs.length === 0) continue;
      const row = document.createElement('div');
      row.className = 'row';
      for (const td of techs) {
        const reqsOk = td.requires.every((r) => ownedTechs.has(r));
        const btn = document.createElement('button');
        btn.textContent = `${tn(`tech.${td.key}`)} · ${techCostLabel(td)}`;
        btn.disabled =
          !reqsOk ||
          td.era > era ||
          pl === undefined ||
          !researchAffordable(td) ||
          (pl.techCur !== 0 && pl.techCur !== td.id);
        btn.onclick = () => issueResearch(td.id);
        row.appendChild(btn);
      }
      panelEl.appendChild(row);
    }

    // current research progress
    if (pl && pl.techCur !== 0) {
      const td = techById.get(pl.techCur);
      if (td) {
        const pct = Math.round(100 * (1 - pl.techLeft / td.ticks));
        const line = document.createElement('div');
        line.className = 'dim';
        line.textContent = `${tn(`tech.${td.key}`)} · ${t('panel.researching', { pct })}`;
        panelEl.appendChild(line);
      }
    }

    // age up button
    if (pl && era < 4) {
      const next = eraDefs[era + 1];
      if (pl.ageLeft > 0) {
        const pct = Math.round(100 * (1 - pl.ageLeft / next.ageTicks));
        const line = document.createElement('div');
        line.textContent = `${t('panel.aging', { era: tn(`era.${next.key}`) })} · ${pct}%`;
        panelEl.appendChild(line);
      } else {
        const req = defById.get(next.reqBuilding);
        const hasReq = buildings.some(
          (b) => b.owner === LOCAL_PLAYER && b.def === next.reqBuilding && b.progress === 0,
        );
        const affordable = pl.res[3] >= next.costFood && pl.res[4] >= next.costGold;
        const btn = document.createElement('button');
        btn.textContent = `${t('panel.ageup', { era: tn(`era.${next.key}`) })} · ${next.costFood}f ${next.costGold}g`;
        btn.disabled = !hasReq || !affordable;
        btn.title = req && !hasReq ? tn(`building.${req.key}`) : '';
        btn.onclick = () => issueAgeUp(era + 1);
        const row = document.createElement('div');
        row.className = 'row';
        row.appendChild(btn);
        panelEl.appendChild(row);
      }
    }
  }

  function setPlacing(def: BuildingDef | null): void {
    placing = def;
    ghost.visible = def !== null;
    for (const el of panelEl.querySelectorAll('[data-hint]')) {
      el.textContent = def ? t('panel.place') : '';
    }
  }

  // --- pointer input --------------------------------------------------------
  let leftStart: { x: number; y: number } | null = null;
  let rightStart: { x: number; yaw: number } | null = null;
  let lastTile: number | null = null;

  canvas.addEventListener('contextmenu', (e) => e.preventDefault());

  canvas.addEventListener('pointerdown', (e) => {
    canvas.setPointerCapture(e.pointerId);
    if (e.button === 0) {
      leftStart = { x: e.clientX, y: e.clientY };
    } else if (e.button === 2) {
      rightStart = { x: e.clientX, yaw: rtsCam.yaw };
    }
  });

  window.addEventListener('pointermove', (e) => {
    if (rightStart) {
      rtsCam.rotate((e.clientX - rightStart.x) * 0.005);
      rightStart.x = e.clientX;
    }
    if (leftStart) {
      const dx = e.clientX - leftStart.x;
      const dy = e.clientY - leftStart.y;
      if (dx * dx + dy * dy > 16) {
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
      }
      return;
    }
    if (e.button !== 0 || !leftStart) return;
    const dragged =
      Math.abs(e.clientX - leftStart.x) > 4 || Math.abs(e.clientY - leftStart.y) > 4;
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
    leftStart = null;
  });

  window.addEventListener('pointerup', (e) => {
    if (e.button !== 2) return;
    if (placing) {
      setPlacing(null);
      return;
    }
    const tile = tileAt(e.clientX, e.clientY);
    if (tile === null) return;
    const enemy = enemyUnitAt(e.clientX, e.clientY, 22);
    if (enemy) {
      issueAttack(enemy);
      return;
    }
    if (e.shiftKey && selectedUnits().length > 0) {
      issuePatrol(tile);
      return;
    }
    const node = gatherTargetAt(tile);
    if (node) {
      issueGather(node);
    } else {
      issueMove(tile);
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
    const node = nodeByTile.get(tile);
    if (node && node.kind !== 5) {
      hoverEl.textContent =
        node.amount > 0
          ? `${tn(KIND_NAMES[node.kind])} · ${node.amount}`
          : t('node.depleted');
    } else {
      const tx = tile % size;
      const ty = Math.floor(tile / size);
      hoverEl.textContent = `${terrainName(tiles[tile])} · ${t('hud.height')} ${heights[ty * (size + 1) + tx]}`;
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

  function renderHud(): void {
    const p = economy[LOCAL_PLAYER];
    if (!p) return;
    resEls.wood.textContent = String(p.res[1]);
    resEls.stone.textContent = String(p.res[2]);
    resEls.food.textContent = String(p.res[3]);
    resEls.gold.textContent = String(p.res[4]);
    resEls.pop.textContent = `${p.pop}/${p.popCap}`;
    const ed = eraDefs[p.era];
    resEls.era.textContent = ed ? tn(`era.${ed.key}`) : String(p.era);
    resEls.relics.textContent = String(relics.filter((r) => r.owner === LOCAL_PLAYER).length);
  }

  // --- main loop ----------------------------------------------------------
  let smoothedMs = 16;
  let frameStart = performance.now();
  let lastLoggedTick = -1;
  let lastFogTick = -1;

  function commitSnapshot(): void {
    units = world.units();
    interp.commit(units.map((u) => [u.id, { x: u.x, y: u.y }] as [number, { x: number; y: number }]));
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
      smoothedMs = smoothedMs * 0.9 + (now - frameStart) * 0.1;
      frameStart = now;

      const alpha = loop.alpha();
      const renderUnits: RenderUnit[] = [];
      for (const u of units) {
        const mine = u.owner === LOCAL_PLAYER;
        if (!mine && !fogView.visible(Math.floor(u.x), Math.floor(u.y))) continue;
        const p = interp.get(u.id, alpha) ?? { x: u.x, y: u.y };
        const yOff = MOVING_STATES.has(u.state)
          ? Math.abs(Math.sin(now * 0.012 + u.id)) * 0.09
          : 0;
        const def = udefById.get(u.def);
        const color =
          u.owner === NEUTRAL_OWNER
            ? u.def === 6
              ? PALETTE.boar
              : PALETTE.deer
            : PLAYER_COLORS[u.owner % PLAYER_COLORS.length];
        const hpFrac = def && def.hp > 0 ? u.hp / def.hp : 1;
        renderUnits.push({
          id: u.id,
          owner: u.owner,
          x: p.x,
          y: p.y,
          yOff,
          carried: u.carried,
          def: u.def,
          cls: def?.class ?? 1,
          color,
          hpFrac,
        });
      }
      actors.setUnits(renderUnits);
      buildings = world.buildings().filter(
        (b) => b.owner === LOCAL_PLAYER || fogView.visible(b.tileX, b.tileY),
      );
      actors.setBuildings(buildings, defById);
      // Refresh every frame so rings track interpolated movement.
      actors.setSelection(selection.toArray());

      const tick = world.tick();
      if (tick % 10 === 0) {
        nodes = world.nodes();
        nodesObj.update(nodes);
        refreshNodeIndex();
        economy = world.economy();
        relics = world.relics();
        ownedTechs = new Set(world.playerTechs(LOCAL_PLAYER));
        renderHud();
        renderPanel();
        const vic = world.victory();
        if (vic.winner !== 255) {
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
        affordable = canAfford(placing);
        ghost.visible = true;
        ghost.position.set(tx, groundY(tx, ty) + 0.05, ty);
        ghost.scale.set(placing.size, 1, placing.size);
        ghostMat.color.setHex(affordable ? PALETTE.leaf : 0xd94f4f);
      }

      driveCamera();
      rtsCam.update();
      ground.cull(camera);
      minimap.draw(fogView, renderUnits, {
        x: rtsCam.target.x,
        z: rtsCam.target.z,
        view: Math.min(size, rtsCam.dist * 1.2),
      }, relics);
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
