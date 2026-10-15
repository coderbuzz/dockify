// Dockify UI check (S7, K8): screenshots, overflow, DOM nodes, navigation timing, contrast, quick a11y.
// Usage: node tests/e2e/ui.mjs [BASE_URL] [OUT]   (env: UI_USER/UI_PASS when auth is on, PW_PATH, UI_RUNS)
// Needs Playwright (default ~/.local/share/pw-tools/node_modules/playwright). Seeds "ZZ Uji" data when missing.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { homedir } from 'node:os';

const BASE = (process.argv[2] || process.env.BASE_URL || 'http://127.0.0.1:18090').replace(/\/$/, '');
const OUT = process.argv[3] || process.env.OUT || '/tmp/claude-1000/dockify-ui';
const RUNS = Number(process.env.UI_RUNS || 5);
const PW = process.env.PW_PATH || join(homedir(), '.local/share/pw-tools/node_modules/playwright/index.mjs');
const CSS = join(dirname(fileURLToPath(import.meta.url)), '../../internal/http/static/app.css');
if (!/^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(BASE)) { console.error('Refusing to run against ' + BASE + ': this script seeds and deletes data, use a local test server.'); process.exit(2); }
const { chromium } = await import(PW);
mkdirSync(OUT, { recursive: true });

const failures = [];
const fail = (m) => { failures.push(m); console.error('FAIL', m); };
const md = (head, rows) => `| ${head.join(' | ')} |\n|${head.map(() => ' --- ').join('|')}|\n` + rows.map((r) => `| ${r.join(' | ')} |`).join('\n') + '\n';
const median = (a) => { const s = [...a].sort((x, y) => x - y); return s[Math.floor(s.length / 2)]; };

const br = await chromium.launch();
async function newCtx(opts = {}) {
  const ctx = await br.newContext({ viewport: { width: 390, height: 844 }, ...opts });
  const p = await ctx.newPage();
  if (process.env.UI_PASS) {
    await p.goto(BASE + '/login');
    if (await p.locator('input[name=user]').count()) {
      await p.fill('input[name=user]', process.env.UI_USER || 'admin');
      await p.fill('input[name=pass]', process.env.UI_PASS);
      await Promise.all([p.waitForNavigation(), p.click('button[type=submit]')]);
    }
  }
  return [ctx, p];
}

let seedState = null; // storage state of a logged-in context, for cleanup
let servers = [], apps = [];
let exitCode = 0;
try {
// ---------- seed ----------
async function post(p, path, form) {
  return p.evaluate(async ([u, f]) => (await fetch(u, { method: 'POST', body: new URLSearchParams(f), redirect: 'manual' })).status, [BASE + path, form]);
}
async function rows(p, path) {
  await p.goto(BASE + path);
  return p.evaluate(() => [...document.querySelectorAll('tr[data-name]')].map((r) => ({ name: r.dataset.name, href: r.querySelector('a').getAttribute('href') })));
}
let [seedCtx, sp] = await newCtx();
seedState = await seedCtx.storageState();
await sp.goto(BASE + '/');
servers = (await rows(sp, '/servers')).filter((r) => r.name.startsWith('ZZ Uji'));
if (!servers.length) {
  for (const n of [1, 2]) await post(sp, '/servers/add', { name: `ZZ Uji server ${n}`, host: `10.9.0.${n}`, user: 'root', port: '22' });
  servers = (await rows(sp, '/servers')).filter((r) => r.name.startsWith('ZZ Uji'));
  for (const s of servers) await post(sp, '/' + s.href + '/init', {});
}
apps = (await rows(sp, '/apps')).filter((r) => r.name.startsWith('ZZ Uji'));
if (!apps.length) {
  const sid = servers[0].href.split('/').pop();
  const base = { port: '3000', server_id: sid, mode: 'simple', image: 'nginx:latest', git_branch: 'main' };
  await post(sp, '/apps/add', { ...base, name: 'ZZ Uji app 1', domain: 'one.example.com' });
  await post(sp, '/apps/add', { ...base, name: 'ZZ Uji app 2', domain: 'two.example.com' });
  await post(sp, '/apps/add', { ...base, name: 'ZZ Uji draft', domain: 'draft.example.com', draft: '1' });
  await sp.waitForTimeout(1500);
  apps = (await rows(sp, '/apps')).filter((r) => r.name.startsWith('ZZ Uji'));
}
await seedCtx.close();
const appRun = apps.find((a) => a.name.endsWith('app 1')) || apps[0];
const appDraft = apps.find((a) => a.name.endsWith('draft')) || apps[apps.length - 1];
const PAGES = [
  ['dashboard', '/'], ['servers', '/servers'], ['apps', '/apps'], ['app-detail', '/' + appRun.href],
  ['app-draft', '/' + appDraft.href], ['server-detail', '/' + servers[0].href], ['apps-add', '/apps/add'], ['settings', '/settings'],
];

// ---------- screenshots, overflow, nodes ----------
const VIEWS = [[360, 740], [390, 844], [1280, 800]];
const nodes = {}, overflow = [];
async function shot(p, name, w, theme, pal) {
  await p.waitForTimeout(250);
  await p.screenshot({ path: join(OUT, `${name}-${w}-${theme}-${pal}.png`) });
}
for (const [w, h] of VIEWS) for (const theme of ['light', 'dark']) {
  const [ctx, p] = await newCtx({ viewport: { width: w, height: h }, colorScheme: theme });
  for (const [name, path] of PAGES) {
    await p.goto(BASE + path);
    await shot(p, name, w, theme, 'indigo');
    const m = await p.evaluate(() => ({
      sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth,
      n: document.querySelectorAll('*').length,
      row: (() => { const r = document.querySelector('table.rows tbody tr'); return r ? r.querySelectorAll('*').length + 1 : 0; })(),
    }));
    if (w < 700 && m.sw > m.cw) { overflow.push(`${name}@${w}`); fail(`overflow ${name} @${w} (${m.sw}>${m.cw})`); }
    if (w === 390 && theme === 'light') nodes[name] = m;
  }
  await ctx.close();
}
// login (unauthenticated)
for (const theme of ['light', 'dark']) {
  const ctx = await br.newContext({ viewport: { width: 390, height: 844 }, colorScheme: theme });
  const p = await ctx.newPage();
  await p.goto(BASE + '/login');
  const isLogin = await p.locator('input[name=user]').count();
  if (isLogin) await shot(p, 'login', 390, theme, 'indigo'); else if (theme === 'light') console.log('login: auth disabled (redirect), skipped; set DOCKIFY_ADMIN_PASSWORD on the test server');
  await ctx.close();
}
// other palettes
for (const pal of ['navy', 'teal']) {
  const ctx = await br.newContext({ viewport: { width: 390, height: 844 }, colorScheme: 'dark' });
  await ctx.addInitScript((v) => { localStorage.setItem('dockify_palette', v); localStorage.setItem('dockify_theme', 'dark'); }, pal);
  const [, p] = [null, await ctx.newPage()];
  if (process.env.UI_PASS) { await p.goto(BASE + '/login'); if (await p.locator('input[name=user]').count()) { await p.fill('input[name=user]', process.env.UI_USER || 'admin'); await p.fill('input[name=pass]', process.env.UI_PASS); await Promise.all([p.waitForNavigation(), p.click('button[type=submit]')]); } }
  for (const [name, path] of [PAGES[0], PAGES[3]]) { await p.goto(BASE + path); await shot(p, name, 390, 'dark', pal); }
  await ctx.close();
}

// ---------- flows with auth on: forms must not hit /logout (regression A1); boosted, so wait for the URL, not a document navigation ----------
{
  const [ctx, p] = await newCtx();
  const sid = servers[0].href.split('/').pop();
  const sid2 = (servers[1] || servers[0]).href.split('/').pop();
  const okUrl = (step) => { const u = p.url(); if (/\/login|\/logout/.test(u) || !/\/apps\/\d+(\?.*)?$/.test(u)) fail(`flow ${step} ended at ${u}`); };
  await p.goto(BASE + '/apps/add');
  await p.fill('[name=name]', 'ZZ Uji flow');
  await p.fill('[name=image]', 'nginx:alpine');
  await p.fill('[name=port]', '8080');
  await p.selectOption('#server-select', sid);
  await Promise.all([p.waitForURL((u) => /\/apps\/\d+$/.test(u.pathname)) , p.click('button:has-text("Save as Draft")')]);
  okUrl('save-as-draft');
  const edit = '/' + appRun.href + '/edit';
  await p.goto(BASE + edit);
  await p.click('button:has-text("Save & Redeploy")');
  await Promise.all([p.waitForURL((u) => /\/apps\/\d+$/.test(u.pathname)) , p.click('#save-redeploy-ok')]);
  okUrl('save-and-redeploy');
  await p.goto(BASE + edit);
  const cur = await p.inputValue('#server-select');
  await p.selectOption('#server-select', cur === sid ? sid2 : sid);
  await p.click('button:has-text("Save & Redeploy")');
  await p.click('#save-redeploy-ok');
  await p.waitForSelector('#move-modal[open]', { timeout: 5000 }).catch(() => fail('flow move: move sheet did not open'));
  await Promise.all([p.waitForURL((u) => /\/apps\/\d+$/.test(u.pathname)) , p.click('button:has-text("Move app")')]);
  okUrl('move-app');
  await ctx.close();
}

// ---------- transfer size (cold) ----------
const transfer = {};
const [lc] = await newCtx();
const state = await lc.storageState();
await lc.close();
for (const [name, path] of PAGES.slice(0, 4)) {
  const ctx = await br.newContext({ viewport: { width: 390, height: 844 }, storageState: state }); // fresh cache
  const p = await ctx.newPage();
  const list = [];
  p.on('request', (rq) => list.push(rq));
  await p.goto(BASE + path, { waitUntil: 'networkidle' }).catch(() => p.waitForTimeout(1500));
  await p.waitForTimeout(300);
  let bytes = 0;
  for (const rq of list) { const z = await rq.sizes().catch(() => null); if (z) bytes += z.responseBodySize + z.responseHeadersSize; }
  transfer[name] = { kb: Math.round(bytes / 1024), reqs: list.length };
  await ctx.close();
}

// ---------- navigation timing (CPU 4x, +80 ms per request) ----------
const timing = {};
{
  const [ctx, p] = await newCtx();
  const cdp = await ctx.newCDPSession(p);
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });
  await p.route('**/*', async (route) => { await new Promise((s) => setTimeout(s, 80)); route.continue().catch(() => {}); });
  for (const [name, path] of [PAGES[0], PAGES[1], PAGES[2], PAGES[3]]) {
    await p.goto(BASE + '/settings'); // navigate from another page
    await p.goto(BASE + path); // warm cache
    const dcl = [], load = [];
    for (let i = 0; i < RUNS; i++) {
      await p.goto(BASE + '/settings');
      await p.goto(BASE + path, { waitUntil: 'load' });
      const t = await p.evaluate(() => { const n = performance.getEntriesByType('navigation')[0]; return [n.domContentLoadedEventEnd, n.loadEventEnd]; });
      dcl.push(Math.round(t[0])); load.push(Math.round(t[1]));
    }
    timing[name] = { dcl: median(dcl), load: median(load) };
  }
  await ctx.close();
}

// ---------- boosted navigation (S9): tab click -> htmx swap of #main, no full reload ----------
const boost = {};
{
  const [ctx, p] = await newCtx();
  const cdp = await ctx.newCDPSession(p);
  // CDP network emulation (not page.route) so the HTTP cache still works for the preload check.
  await cdp.send('Network.enable');
  await cdp.send('Network.emulateNetworkConditions', { offline: false, latency: 80, downloadThroughput: -1, uploadThroughput: -1 });
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });
  await p.goto(BASE + '/servers');
  await p.evaluate(() => {
    window.__hdr = document.querySelector('header.top-nav');
    window.__t = {};
    document.addEventListener('htmx:beforeRequest', (e) => { if (!(e.detail.requestConfig.headers || {})['HX-Preloaded']) window.__t.start = performance.now(); });
    document.addEventListener('htmx:afterSettle', () => { window.__t.end = performance.now(); });
  });
  const state = () => p.evaluate(() => ({
    title: document.title, cur: document.querySelector('.main-nav [aria-current]')?.getAttribute('data-nav'),
    path: location.pathname, hdr: window.__hdr.isConnected, nodes: document.querySelectorAll('*').length, y: scrollY,
  }));
  async function go(tab, preload) { // returns ms from htmx:beforeRequest to htmx:afterSettle
    await p.evaluate(() => { window.__t = {}; });
    const sel = `.main-nav a[data-nav=${tab}]`;
    if (preload) { await p.dispatchEvent(sel, 'touchstart'); await p.waitForTimeout(700); }
    await p.evaluate((q) => document.querySelector(q).click(), sel);
    await p.waitForFunction(() => window.__t.end, null, { timeout: 8000 });
    return p.evaluate(() => Math.round(window.__t.end - window.__t.start));
  }
  for (const preload of [false, true]) {
    const t = [];
    for (let i = 0; i < RUNS; i++) { await go('servers', false); t.push(await go('apps', preload)); }
    boost[preload ? 'preload' : 'plain'] = median(t);
  }
  if (boost.preload > 150) fail(`boosted nav with preload ${boost.preload} ms > 150`);
  if (boost.plain > 250) fail(`boosted nav without preload ${boost.plain} ms > 250`);
  // state after a boosted swap
  await go('servers', false); await p.evaluate(() => scrollTo(0, 400));
  await go('apps', false);
  const st = await state();
  if (st.cur !== 'apps' || !/^Apps/.test(st.title) || !/\/apps$/.test(st.path)) fail(`boost state wrong: ${JSON.stringify(st)}`);
  if (!st.hdr) fail('header was re-rendered by boosted navigation');
  if (st.y !== 0) fail(`boost did not scroll to top (y=${st.y})`);
  // back / forward
  await p.goBack(); await p.waitForFunction(() => /\/servers$/.test(location.pathname)); await p.waitForTimeout(400);
  const back = await state();
  if (back.cur !== 'servers' || !/^Servers/.test(back.title)) fail(`back: ${JSON.stringify(back)}`);
  await p.goForward(); await p.waitForFunction(() => /\/apps$/.test(location.pathname)); await p.waitForTimeout(400);
  const fwd = await state();
  if (fwd.cur !== 'apps') fail(`forward: ${JSON.stringify(fwd)}`);
  // leak check: 10x list <-> app detail (live charts + WebSocket) must not grow the DOM or the cleanup list
  await go('apps', false);
  const before = (await state()).nodes;
  for (let i = 0; i < 10; i++) {
    await p.evaluate((h) => document.querySelector(`a[href="${h}"]`).click(), appRun.href);
    await p.waitForSelector('#app-stats-body, .gauge', { timeout: 8000 });
    await p.waitForTimeout(300);
    await go('apps', false);
  }
  const after = await state();
  const live = await p.evaluate(() => (window.dockifyCleanups || []).length);
  boost.leak = { before, after: after.nodes, cleanups: live };
  if (after.nodes > before + 5) fail(`DOM nodes grew after 10 boosted round trips: ${before} -> ${after.nodes}`);
  if (live > 0) fail(`dockifyCleanups not empty on a list page: ${live}`);
  // detail page: charts script still ran after the swap
  await p.evaluate((h) => document.querySelector(`a[href="${h}"]`).click(), appRun.href);
  await p.waitForTimeout(2500);
  const charts = await p.evaluate(() => ({ cleanups: (window.dockifyCleanups || []).length, line: !!document.querySelector('[id$="-chart-line"]') }));
  if (charts.cleanups < 1) fail('live charts were not initialised after a boosted navigation');
  boost.detail = charts;
  // login -> logout -> login again (these are never boosted)
  await p.click('button[aria-label="Sign out"]');
  await p.waitForURL(/\/login$/);
  await p.fill('input[name=user]', process.env.UI_USER || 'admin');
  await p.fill('input[name=pass]', process.env.UI_PASS || '');
  await Promise.all([p.waitForURL((u) => !/\/login/.test(u.pathname)), p.click('button[type=submit]')]);
  boost.relogin = await p.evaluate(() => document.title);
  await ctx.close();
}

// ---------- contrast ----------
const css = readFileSync(CSS, 'utf8');
const lum = (hex) => { const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4)); return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]; };
const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
const PAIRS = [
  ['ink', 'bg', 4.5], ['ink', 'surface', 4.5], ['ink2', 'surface', 4.5], ['muted', 'surface', 4.5], ['control', 'surface', 3],
  ['on-accent', 'accent-fill', 4.5], ['accent', 'surface', 4.5], ['danger', 'surface', 4.5], ['danger-text', 'danger-bg', 4.5],
  ['ok-text', 'ok-bg', 4.5], ['warn-text', 'warn-bg', 4.5], ['on-brand2', 'brand2-fill', 4.5], ['muted', 'bg', 4.5],
  ['accent', 'accent-soft', 4.5], ['ink2', 'sunk', 4.5], ['ok', 'surface', 4.5], ['on-accent', 'danger-fill', 4.5],
  ['brand2', 'surface', 4.5], ['chart', 'chart-soft', 3],
];
function tokens(sel) {
  const m = css.match(new RegExp('^' + sel.replace(/[[\]]/g, '\\$&') + ' \\{([^}]*)\\}', 'm'));
  if (!m) throw new Error('token block not found: ' + sel);
  return Object.fromEntries([...m[1].matchAll(/--([a-z0-9-]+):\s*(#[0-9a-fA-F]{6})/g)].map((x) => [x[1], x[2]]));
}
const contrast = [];
for (const pal of ['indigo', 'navy', 'teal']) for (const theme of ['light', 'dark']) {
  const t = tokens(`:root[data-palette=${pal}]` + (theme === 'dark' ? '[data-theme=dark]' : ''));
  let bad = 0;
  for (const [a, b, min] of PAIRS) { const r = ratio(t[a], t[b]); if (r < min) { bad++; fail(`contrast ${pal}/${theme} ${a} on ${b} = ${r.toFixed(2)} < ${min}`); } }
  contrast.push([pal, theme, `${PAIRS.length - bad}/${PAIRS.length}`]);
}

// ---------- a11y quick ----------
const a11y = [];
{
  const [ctx, p] = await newCtx();
  for (const [name, path] of PAGES) {
    await p.goto(BASE + path);
    const r = await p.evaluate(() => {
      const unnamed = [...document.querySelectorAll('button, a')].filter((e) => !e.closest('template') && !e.hidden && !e.textContent.trim() && !e.getAttribute('aria-label') && !e.getAttribute('aria-labelledby')).map((e) => e.outerHTML.slice(0, 80));
      const dlg = [...document.querySelectorAll('dialog')].filter((d) => !d.getAttribute('aria-labelledby')).map((d) => d.id);
      return { unnamed, dlg };
    });
    await p.evaluate(() => document.activeElement && document.activeElement.blur());
    await p.keyboard.press('Tab');
    const focus = await p.evaluate(() => { const e = document.activeElement; if (!e || e === document.body) return 'none'; const s = getComputedStyle(e); return s.outlineStyle !== 'none' && parseFloat(s.outlineWidth) > 0 ? 'ok' : 'no-outline'; });
    const bad = r.unnamed.length + r.dlg.length + (focus === 'ok' ? 0 : 1);
    if (bad) fail(`a11y ${name}: unnamed=${r.unnamed.length} dialogs=${r.dlg.join(',')} focus=${focus}`);
    a11y.push([name, r.unnamed.length, r.dlg.length, focus]);
  }
  await ctx.close();
}

// ---------- report ----------
const rep = [`# Dockify UI report\n\nBase: ${BASE} · ${new Date().toISOString()} · runs=${RUNS}\n`,
  '## Navigation (CPU 4x, +80 ms per request, median)\n', md(['page', 'DOMContentLoaded ms', 'load ms'], Object.entries(timing).map(([k, v]) => [k, v.dcl, v.load])),
  '\n## Boosted navigation (S9, tab click, CPU 4x, +80 ms, median)\n', md(['case', 'ms'], [['with preload (touchstart first)', boost.preload], ['without preload', boost.plain]]),
  `\nLeak check (10 round trips list <-> detail): nodes ${boost.leak?.before} -> ${boost.leak?.after}, cleanups left ${boost.leak?.cleanups}\n`,
  '\n## Transfer (cold, on the wire) and DOM nodes @390\n', md(['page', 'KB', 'requests', 'DOM nodes', 'nodes/row'], Object.keys(nodes).map((k) => [k, transfer[k]?.kb ?? '-', transfer[k]?.reqs ?? '-', nodes[k].n, nodes[k].row])),
  `\n## Overflow @360/390\n\n${overflow.length ? overflow.join(', ') : 'none (0)'}\n`,
  '\n## Contrast (WCAG, 19 pairs)\n', md(['palette', 'theme', 'pass'], contrast),
  '\n## A11y quick\n', md(['page', 'unnamed controls', 'dialogs w/o label', 'first Tab focus'], a11y),
  `\n## Result\n\n${failures.length ? 'FAIL\n\n' + failures.map((f) => '- ' + f).join('\n') : 'PASS'}\n`].join('\n');
writeFileSync(join(OUT, 'report.md'), rep);
console.log(rep);
exitCode = failures.length ? 1 : 0;
} finally {
  // remove the "ZZ Uji" data even when a check failed
  try {
    if (seedState) {
      const c = await br.newContext({ storageState: seedState });
      const p = await c.newPage();
      await p.goto(BASE + '/apps');
      const list = await p.evaluate(() => [...document.querySelectorAll('tr[data-name]')].filter((r) => r.dataset.name.startsWith('ZZ Uji')).map((r) => r.querySelector('a').getAttribute('href')));
      for (const href of list) await p.evaluate((u) => fetch(u + '/undeploy', { method: 'POST' }), BASE + '/' + href);
      await p.waitForTimeout(1500);
      await p.goto(BASE + '/servers');
      const sl = await p.evaluate(() => [...document.querySelectorAll('tr[data-name]')].filter((r) => r.dataset.name.startsWith('ZZ Uji')).map((r) => r.querySelector('a').getAttribute('href')));
      for (const href of sl) await p.evaluate((u) => fetch(u + '/delete', { method: 'POST' }), BASE + '/' + href);
      await c.close();
    }
  } catch (e) { console.error('cleanup failed:', e.message); }
  await br.close().catch(() => {});
}
process.exit(exitCode);
