import * as THREE from 'three';

import { SimWorld, seedFromLocation } from './sim/bridge';
import { loadSim } from './sim/loader';
import { FixedStepLoop } from './engine/loop';
import { Selection, normalizeRect, selectInRect, type ScreenPoint } from './engine/selection';
import { createScene } from './render/scene';
import { buildTerrainMesh } from './render/terrainMesh';
import { buildMapObjects } from './render/objects';
import { RtsCamera } from './render/camera';
import { terrainName } from './render/terrainLabels';
import { t } from './i18n';

const statusEl = document.querySelector<HTMLDivElement>('#status')!;
const statsEl = document.querySelector<HTMLDivElement>('#stats')!;
const tickEl = document.querySelector<HTMLSpanElement>('#tick')!;
const hashEl = document.querySelector<HTMLSpanElement>('#hash')!;
const mapEl = document.querySelector<HTMLSpanElement>('#map')!;
const fpsEl = document.querySelector<HTMLSpanElement>('#fps')!;
const selectedEl = document.querySelector<HTMLSpanElement>('#selected')!;
const hoverEl = document.querySelector<HTMLDivElement>('#hover')!;
const selRectEl = document.querySelector<HTMLDivElement>('#selrect')!;
const canvas = document.querySelector<HTMLCanvasElement>('#gl')!;

statusEl.textContent = t('hud.status.loading');

try {
  const sim = await loadSim();
  const seed = seedFromLocation(window.location.search);
  const world = SimWorld.create(sim, seed);
  statusEl.hidden = true;
  statsEl.hidden = false;
  console.info(t('hud.status.ready', { version: sim.version(), seed: world.seed() }));

  const size = world.mapSize();
  const tiles = world.mapTiles();
  const heights = world.vertexHeights();
  console.info(`map ${size}x${size} hash=${world.mapHashHex()}`);

  const { renderer, scene, camera } = createScene(canvas, size);
  scene.add(buildTerrainMesh(size, tiles, heights));
  const objects = buildMapObjects(size, tiles, heights, world.spawns(), seed ^ 0xa17c);
  scene.add(objects.group);
  const rtsCam = new RtsCamera(camera, size);

  mapEl.textContent = `${size}² ${world.mapHashHex().slice(0, 8)}`;

  // --- selection -------------------------------------------------------
  const selection = new Selection();
  const byTile = new Map<number, number>();
  for (const f of objects.features) byTile.set(f.id, f.id);

  function applySelection(): void {
    objects.setSelection(selection.toArray());
    selectedEl.textContent = String(selection.size);
  }

  // --- picking ----------------------------------------------------------
  const raycaster = new THREE.Raycaster();
  const groundMesh = scene.children.find((c) => c instanceof THREE.Mesh)!;

  function tileAt(clientX: number, clientY: number): number | null {
    const ndc = new THREE.Vector2(
      (clientX / window.innerWidth) * 2 - 1,
      -(clientY / window.innerHeight) * 2 + 1,
    );
    raycaster.setFromCamera(ndc, camera);
    const hits = raycaster.intersectObject(groundMesh, false);
    if (hits.length === 0) return null;
    const tx = Math.floor(hits[0].point.x);
    const ty = Math.floor(hits[0].point.z);
    if (tx < 0 || ty < 0 || tx >= size || ty >= size) return null;
    return ty * size + tx;
  }

  function featureScreenPoints(): ScreenPoint[] {
    const v = new THREE.Vector3();
    return objects.features.map((f) => {
      v.copy(f.position).project(camera);
      return {
        id: f.id,
        sx: (v.x * 0.5 + 0.5) * window.innerWidth,
        sy: (-v.y * 0.5 + 0.5) * window.innerHeight,
        onScreen: v.z >= -1 && v.z <= 1,
      };
    });
  }

  // --- pointer input ----------------------------------------------------
  let leftStart: { x: number; y: number } | null = null;
  let rightStart: { x: number; yaw: number } | null = null;

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
      rightStart = null;
      return;
    }
    if (e.button !== 0 || !leftStart) return;
    const dragged =
      Math.abs(e.clientX - leftStart.x) > 4 || Math.abs(e.clientY - leftStart.y) > 4;
    selRectEl.style.display = 'none';

    if (dragged) {
      const rect = normalizeRect(leftStart.x, leftStart.y, e.clientX, e.clientY);
      selection.replace(selectInRect(featureScreenPoints(), rect));
    } else {
      const tile = tileAt(e.clientX, e.clientY);
      if (tile !== null && byTile.has(tile)) {
        selection.replace([tile]);
      } else {
        selection.clear();
      }
    }
    applySelection();
    leftStart = null;
  });

  function updateHover(clientX: number, clientY: number): void {
    const tile = tileAt(clientX, clientY);
    if (tile === null) {
      hoverEl.style.visibility = 'hidden';
      return;
    }
    const tx = tile % size;
    const ty = Math.floor(tile / size);
    const h = heights[ty * (size + 1) + tx];
    hoverEl.textContent = `${terrainName(tiles[tile])} · ${t('hud.height')} ${h}`;
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

  // --- main loop ----------------------------------------------------------
  let smoothedMs = 16;
  let frameStart = performance.now();
  let lastLoggedTick = -1;

  const loop = new FixedStepLoop(
    () => world.step(),
    () => {
      const now = performance.now();
      smoothedMs = smoothedMs * 0.9 + (now - frameStart) * 0.1;
      frameStart = now;

      driveCamera();
      rtsCam.update();
      renderer.render(scene, camera);

      const tick = world.tick();
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
