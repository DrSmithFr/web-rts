import * as THREE from 'three';

import { SimWorld, seedFromLocation, type BuildingDef, type BuildingRec, type NodeRec, type UnitRec } from './sim/bridge';
import { loadSim } from './sim/loader';
import { OrderKind, buildPayload, gatherPayload, moveGroupPayload, trainPayload } from './sim/ordercodec';
import { FixedStepLoop } from './engine/loop';
import { Interpolated } from './engine/interpolate';
import { FogView } from './render/fogView';
import { GroundChunks } from './render/terrainChunks';
import { Minimap } from './render/minimap';
import type { RenderUnit } from './render/actors';
import { Selection, normalizeRect, selectInRect, type ScreenPoint } from './engine/selection';
import { createScene } from './render/scene';
import { buildMapObjects } from './render/objects';
import { buildNodeObjects } from './render/nodes';
import { buildActors } from './render/actors';
import { RtsCamera } from './render/camera';
import { HEIGHT_SCALE, PALETTE } from './render/palette';
import { terrainName } from './render/terrainLabels';
import { t, tn } from './i18n';

const LOCAL_PLAYER = 0;
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
};
const canvas = document.querySelector<HTMLCanvasElement>('#gl')!;

statusEl.textContent = t('hud.status.loading');

try {
  const sim = await loadSim();
  const seed = seedFromLocation(window.location.search);
  const world = SimWorld.create(sim, seed);
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

  const rtsCam = new RtsCamera(camera, size, (x, z) =>
    groundY(Math.min(size - 1, Math.max(0, Math.floor(x))), Math.min(size - 1, Math.max(0, Math.floor(z)))));
  const home = world.spawns()[LOCAL_PLAYER] ?? { x: size / 2, y: size / 2 };
  rtsCam.focus(home.x + 0.5, home.y + 0.5);
  rtsCam.zoom(0.5);
  mapEl.textContent = `${size}² ${world.mapHashHex().slice(0, 8)}`;

  let units: UnitRec[] = [];
  let buildings: BuildingRec[] = [];
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
  function issueMove(tile: number): void {
    const ids = selectedVillagers();
    if (ids.length === 0) return;
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    world.order(LOCAL_PLAYER, OrderKind.MoveGroup, moveGroupPayload(ids, tx * 16 + 8, ty * 16 + 8));
  }

  function issueGather(node: NodeRec): void {
    for (const id of selectedVillagers()) {
      world.order(LOCAL_PLAYER, OrderKind.Gather, gatherPayload(id, node.id));
    }
  }

  function issueBuild(tile: number): void {
    if (!placing) return;
    const ids = selectedVillagers();
    if (ids.length === 0) return;
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    world.order(LOCAL_PLAYER, OrderKind.Build, buildPayload(ids[0], placing.id, tx, ty));
    setPlacing(null);
  }

  function issueTrain(buildingId: number): void {
    world.order(LOCAL_PLAYER, OrderKind.Train, trainPayload(buildingId));
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

  function renderPanel(): void {
    const villagers = selectedVillagers().length;
    const selBuildings = selection
      .toArray()
      .map((id) => buildings.find((b) => b.id === id))
      .filter((b): b is BuildingRec => b !== undefined && b.owner === LOCAL_PLAYER);

    if (villagers === 0 && selBuildings.length === 0) {
      panelEl.style.display = 'none';
      return;
    }
    panelEl.style.display = 'block';
    panelEl.textContent = '';

    if (villagers > 0) {
      const title = document.createElement('div');
      title.textContent = t('panel.villagers', { count: villagers });
      panelEl.appendChild(title);

      const row = document.createElement('div');
      row.className = 'row';
      for (const def of defs) {
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

    for (const b of selBuildings) {
      const def = defById.get(b.def);
      const line = document.createElement('div');
      if (!def) continue;
      if (b.progress > 0) {
        const pct = Math.round(100 * (1 - b.progress / def.buildTicks));
        line.textContent = `${tn(`building.${def.key}`)} · ${t('panel.progress', { pct })}`;
      } else {
        let text = tn(`building.${def.key}`);
        if (def.trainTicks > 0) {
          const queued = b.trainLeft > 0 ? b.trainReq + 1 : b.trainReq;
          text += ` · ${t('panel.training')} ${queued}`;
        }
        line.textContent = text;
      }
      panelEl.appendChild(line);
      if (def.trainTicks > 0 && b.progress === 0) {
        const row = document.createElement('div');
        row.className = 'row';
        const btn = document.createElement('button');
        btn.textContent = `${t('panel.train')} · ${def.trainFood}f`;
        btn.onclick = () => issueTrain(b.id);
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
    const node = gatherTargetAt(tile);
    if (node) {
      issueGather(node);
    } else {
      issueMove(tile);
    }
  });

  window.addEventListener('keydown', (e) => {
    if (e.code === 'Escape') setPlacing(null);
  });

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

  const loop = new FixedStepLoop(
    () => {
      world.step();
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
        const moving = u.state === 1;
        const yOff = moving ? Math.abs(Math.sin(now * 0.012 + u.id)) * 0.09 : 0;
        renderUnits.push({ id: u.id, owner: u.owner, x: p.x, y: p.y, yOff, carried: u.carried });
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
        renderHud();
        renderPanel();
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
      });
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
