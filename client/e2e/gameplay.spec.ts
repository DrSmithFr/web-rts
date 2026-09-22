export {};
import { expect, test } from '@playwright/test';

declare global {
  interface Window {
    __rts: any;
  }
}

// A scripted solo run at turbo speed: harvest berries to the Antiquity
// age-up, build a second hut and train a villager — the core loop the
// spec demands the E2E to cover.
test('build, train and age up from a scripted solo run', async ({ page }) => {
  await page.goto('http://127.0.0.1:5173/?menu=off&bots=0&seed=0x5eed&turbo=40');
  await page.waitForFunction(() => typeof window.__rts !== 'undefined', undefined, { timeout: 30_000 });
  await page.waitForFunction(() => window.__rts.tick() > 10, undefined, { timeout: 15_000 });

  // send every villager to berries
  await page.evaluate(() => {
    const r = window.__rts;
    const food = r.nodes().filter((n: any) => n.kind === 3 && n.amount > 0);
    const vills = r.units().filter((u: any) => u.owner === r.local() && u.kind === 1);
    for (const [i, v] of vills.entries()) {
      r.issue(r.kinds.Gather, r.payload.gather(v.id, food[Math.min(i, food.length - 1)].id));
    }
  });

  // build a hut on open ground near the first villager once wood allows
  await page.waitForFunction(() => {
    const r = window.__rts;
    return r.economy()[r.local()].res[1] >= 60;
  }, undefined, { timeout: 120_000 });
  const built = await page.evaluate(() => {
    const r = window.__rts;
    const v = r.units().find((u: any) => u.owner === r.local() && u.kind === 1);
    const size = r.world.mapSize();
    const vx = Math.floor(v.x / 65536);
    const vy = Math.floor(v.y / 65536);
    for (const [dx, dy] of [[3, 0], [-3, 0], [0, 3], [0, -3], [2, 2], [-2, 2], [2, -2], [-2, -2]]) {
      const tx = vx + dx;
      const ty = vy + dy;
      if (tx < 1 || ty < 1 || tx >= size - 1 || ty >= size - 1) continue;
      r.issue(r.kinds.Build, r.payload.build(v.id, 1, tx, ty));
      return true;
    }
    return false;
  });
  expect(built).toBeTruthy();

  // train a villager at the storehouse
  await page.waitForFunction(() => {
    const r = window.__rts;
    const me = r.local();
    const store = r.world.buildings().find((b: any) => b.owner === me && b.def === 3);
    return !!store && r.economy()[me].res[3] >= 60 && r.economy()[me].pop < r.economy()[me].popCap;
  }, undefined, { timeout: 120_000 });
  await page.evaluate(() => {
    const r = window.__rts;
    const store = r.world.buildings().find((b: any) => b.owner === r.local() && b.def === 3);
    r.issue(r.kinds.Train, r.payload.train(store.id, 1));
  });
  await page.waitForFunction(() => {
    const r = window.__rts;
    return r.economy()[r.local()].pop >= 4;
  }, undefined, { timeout: 120_000 });

  // age up once the berry stash covers Antiquity (250 food)
  await page.waitForFunction(() => {
    const r = window.__rts;
    const e = r.economy()[r.local()];
    return e.era >= 1 || (e.era === 0 && e.res[3] >= 250 && e.ageTarget === 0);
  }, undefined, { timeout: 200_000 });
  await page.evaluate(() => {
    const r = window.__rts;
    if (r.economy()[r.local()].era === 0) r.issue(r.kinds.AgeUp, r.payload.era(1));
  });
  await page.waitForFunction(() => window.__rts.economy()[window.__rts.local()].era >= 1, undefined, {
    timeout: 120_000,
  });
});
