export {};
import { expect, test } from '@playwright/test';

// helpers run INSIDE the page: Playwright serializes these callbacks, so
// they must not close over Node-side variables.
declare global {
  interface Window {
    __rts: {
      issue(kind: number, payload: Uint8Array): void;
      local(): number;
      tick(): number;
      units(): { id: number; owner: number; kind: number }[];
      nodes(): { id: number; kind: number; amount: number }[];
      economy(): { res: number[]; era: number; pop: number }[];
      kinds: Record<string, number>;
      payload: Record<string, (...args: number[]) => Uint8Array>;
    };
  }
}

test('main menu starts a solo game that harvests its first resource', async ({ page }) => {
  await page.goto('http://127.0.0.1:5173/');
  await expect(page.locator('#menu .mtitle:visible')).toHaveText('AGE OF SEEDS');
  await page.getByRole('button', { name: 'Solo game' }).click();
  await page.getByRole('button', { name: 'Start' }).click();

  await page.waitForFunction(() => typeof window.__rts !== 'undefined', undefined, { timeout: 30_000 });
  await expect(page.locator('#menu')).toBeHidden();
  await page.waitForFunction(() => window.__rts.tick() > 20, undefined, { timeout: 15_000 });

  // order every villager to a food node
  const ordered = await page.evaluate(() => {
    const r = window.__rts;
    const food = r.nodes().filter((n) => n.kind === 3 && n.amount > 0);
    if (food.length === 0) return 0;
    const vills = r.units().filter((u) => u.owner === r.local() && u.kind === 1);
    for (const v of vills) r.issue(r.kinds.Gather, r.payload.gather(v.id, food[0].id));
    return vills.length;
  });
  expect(ordered).toBeGreaterThan(0);

  const startFood = await page.evaluate(() => window.__rts.economy()[window.__rts.local()].res[3]);
  await page.waitForFunction(
    (f) => window.__rts.economy()[window.__rts.local()].res[3] > f,
    startFood,
    { timeout: 90_000 },
  );
});
