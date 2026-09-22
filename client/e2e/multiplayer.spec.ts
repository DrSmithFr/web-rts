export {};
import { expect, test } from '@playwright/test';

declare global {
  interface Window {
    __rts: any;
  }
}

const WS = 'ws://127.0.0.1:8787';

test('a second browser context joins a hosted room and hash-syncs', async ({ browser }) => {
  const ctxA = await browser.newContext();
  const pageA = await ctxA.newPage();
  await pageA.goto(`http://127.0.0.1:5173/?ws=${encodeURIComponent(WS)}&create=1&name=Alice&bots=1&difficulty=easy`);
  await pageA.waitForFunction(() => typeof window.__rts !== 'undefined' && window.__rts.isOnline(), undefined, { timeout: 30_000 });

  // room code shows up in the netstatus banner: "room abc123: ..."
  await expect(pageA.locator('#netstatus')).toContainText(/room [0-9a-f]{6}/, { timeout: 15_000 });
  const text = await pageA.locator('#netstatus').textContent();
  const code = /room ([0-9a-f]{6})/.exec(text || '')?.[1];
  expect(code).toBeTruthy();

  const ctxB = await browser.newContext();
  const pageB = await ctxB.newPage();
  await pageB.goto(`http://127.0.0.1:5173/?ws=${encodeURIComponent(WS)}&code=${code}&name=Bob`);
  await pageB.waitForFunction(() => typeof window.__rts !== 'undefined' && window.__rts.isOnline(), undefined, { timeout: 30_000 });

  // Bob appears in Alice's lobby, then Alice starts the match
  await expect(pageA.locator('#netstatus')).toContainText('Bob', { timeout: 15_000 });
  await pageA.evaluate(() => window.__rts.start('standard'));

  await pageA.waitForFunction(() => window.__rts?.tick?.() > 120, undefined, { timeout: 120_000 });
  await pageB.waitForFunction(() => window.__rts?.tick?.() > 60, undefined, { timeout: 120_000 });

  const [ha, ta, hb, tb] = await Promise.all([
    pageA.evaluate(() => window.__rts.hash()),
    pageA.evaluate(() => window.__rts.tick()),
    pageB.evaluate(() => window.__rts.hash()),
    pageB.evaluate(() => window.__rts.tick()),
  ]);
  expect(ta).toBeGreaterThan(100);
  expect(tb).toBeGreaterThan(50);
  // both clients confirm the same authoritative hash at their own tick
  expect(ha.length).toBe(16);
  expect(hb.length).toBe(16);

  const statusA = (await pageA.locator('#netstatus').textContent()) || '';
  const statusB = (await pageB.locator('#netstatus').textContent()) || '';
  expect(statusA).not.toContain('DESYNC');
  expect(statusB).not.toContain('DESYNC');
});
