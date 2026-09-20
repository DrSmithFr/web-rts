import { SimWorld, seedFromLocation } from './sim/bridge';
import { loadSim } from './sim/loader';
import { FixedStepLoop } from './engine/loop';
import { t } from './i18n';

const statusEl = document.querySelector<HTMLDivElement>('#status')!;
const statsEl = document.querySelector<HTMLDivElement>('#stats')!;
const tickEl = document.querySelector<HTMLSpanElement>('#tick')!;
const hashEl = document.querySelector<HTMLSpanElement>('#hash')!;
const fpsEl = document.querySelector<HTMLSpanElement>('#fps')!;

statusEl.textContent = t('hud.status.loading');

try {
  const sim = await loadSim();
  const world = SimWorld.create(sim, seedFromLocation(window.location.search));
  statusEl.hidden = true;
  statsEl.hidden = false;
  console.info(t('hud.status.ready', { version: sim.version(), seed: world.seed() }));

  let smoothedMs = 16;
  let frameStart = performance.now();
  let lastLoggedTick = -1;

  const loop = new FixedStepLoop(
    () => world.step(),
    () => {
      const now = performance.now();
      smoothedMs = smoothedMs * 0.9 + (now - frameStart) * 0.1;
      frameStart = now;

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
