export {};
import { expect, test } from '@playwright/test';

declare global {
  interface Window {
    __rts: any;
  }
}

const FPS_PROBE = `new Promise((resolve) => {
  let frames = 0;
  const start = performance.now();
  const count = () => {
    frames++;
    if (performance.now() - start > 2000) resolve((frames * 1000) / (performance.now() - start));
    else requestAnimationFrame(count);
  };
  requestAnimationFrame(count);
})`;

test('500 units keep the frame rate within 60% of the empty scene', async ({ page }) => {
  await page.goto('http://127.0.0.1:5173/?menu=off&bots=0&seed=0x5eed');
  await page.waitForFunction(() => typeof window.__rts !== 'undefined', undefined, { timeout: 30_000 });
  await page.waitForFunction(() => window.__rts.tick() > 5, undefined, { timeout: 15_000 });

  const fpsBase: number = await page.evaluate(FPS_PROBE);
  // software-GL runners are slow and noisy; only require progress
  expect(fpsBase).toBeGreaterThan(1.5);

  await page.evaluate(() => {
    window.__rts.spawn(window.__rts.local(), 1, 500);
  });
  await page.waitForFunction(() => window.__rts.units().length > 500, undefined, { timeout: 60_000 });

  const fpsWith: number = await page.evaluate(FPS_PROBE);
  // the sim step is O(units), but rendering must not collapse once the
  // scene grows: 500 villagers may cost at most 60% of frame rate
  expect(fpsWith).toBeGreaterThan(fpsBase * 0.4);
});
