// Generator preview 3 palet Dockify (nilai token dari docs/DESIGN-SYSTEM.md §3). Hasil: preview-warna.html (statis, tanpa JS, font data URI) + kontras.md.
// Jalankan: node gen.mjs   (butuh akses ke fonts.googleapis.com untuk menanam font)
import { readFileSync, writeFileSync } from "node:fs";

// ---------- Token lapis 2 per opsi: dibaca dari docs/DESIGN-SYSTEM.md §3 (sumber tunggal nilai final) ----------
const DS = readFileSync(new URL("../DESIGN-SYSTEM.md", import.meta.url), "utf8");
function palet(slug) {
	const sec = DS.split(/^### /m).find((x) => x.startsWith(`3.`) && x.includes("`" + slug + "`"));
	if (!sec) throw new Error(`palet ${slug} tidak ada di DESIGN-SYSTEM.md §3`);
	const light = {}, dark = {};
	for (const m of sec.matchAll(/^\| ([a-z0-9-]+) \| `([^`]+)` \| `([^`]+)` \|$/gm)) [light[m[1]], dark[m[1]]] = [m[2], m[3]];
	return { light, dark };
}
const OPSI = [
	{ id: "1", nama: "1 · Indigo Dockify (default)", catatan: "Netral slate, indigo untuk semua aksi utama (termasuk Deploy App), violet untuk seri grafik kedua.", ...palet("indigo") },
	{ id: "2", nama: "2 · Navy Brosur", catatan: "Navy tua untuk aksi, biru untuk seri grafik kedua, grafik indigo-navy. Paling formal.", ...palet("navy") },
	{ id: "3", nama: "3 · Slate Teal Ops", catatan: "Netral abu-biru dingin, teal gelap untuk aksi, biru langit untuk seri kedua. Kesan layar monitoring.", ...palet("teal") }
];

const ORDER = ["bg", "surface", "sunk", "line", "line-strong", "control", "ink", "ink2", "muted", "accent", "accent-fill", "on-accent", "accent-soft",
	"brand2-fill", "on-brand2", "brand2", "brand2-soft", "chart", "chart-soft", "ok", "ok-text", "ok-bg", "warn", "warn-text", "warn-bg",
	"danger", "danger-text", "danger-bg", "danger-fill", "shadow"];
const tok = (o, mode) => o[mode];
for (const o of OPSI) for (const mode of ["light", "dark"]) {
	const miss = ORDER.filter((k) => !o[mode][k]);
	if (miss.length) throw new Error(`opsi ${o.id} ${mode}: token hilang ${miss.join(", ")}`);
}

// ---------- Kontras WCAG ----------
const lum = (hex) => {
	const c = hex.slice(1).match(/../g).map((h) => parseInt(h, 16) / 255).map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
	return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
};
const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
// [fg, bg, minimum, keterangan]; "#ffffff" = teks putih di tombol destructive.
const PAIRS = [
	["ink", "bg", 4.5], ["ink", "surface", 4.5], ["ink2", "surface", 4.5], ["muted", "surface", 4.5], ["control", "surface", 3],
	["on-accent", "accent-fill", 4.5], ["accent", "surface", 4.5], ["danger", "surface", 4.5], ["danger-text", "danger-bg", 4.5],
	["ok-text", "ok-bg", 4.5], ["warn-text", "warn-bg", 4.5], ["on-brand2", "brand2-fill", 4.5],
	// tambahan: dipakai di mockup
	["muted", "bg", 4.5, "meta di latar halaman"], ["accent", "accent-soft", 4.5, "chip aktif"], ["ink2", "sunk", 4.5, "lencana netral"],
	["ok", "surface", 4.5, "angka KPI Online/Running"], ["#ffffff", "danger-fill", 4.5, "tombol Undeploy di konfirmasi"],
	["brand2", "surface", 4.5, "angka sekunder"], ["chart", "chart-soft", 3, "bar CPU/RAM/Disk (grafis)"]
];
const kontras = [];
let gagal = 0;
for (const o of OPSI) for (const mode of ["light", "dark"]) {
	const t = tok(o, mode);
	for (const [f, b, min, ket] of PAIRS) {
		const r = ratio(f.startsWith("#") ? f : t[f], t[b]);
		const ok = r >= min;
		if (!ok) gagal++;
		kontras.push({ o: o.id, mode, f: f === "#ffffff" ? "white" : f, b, min, r, ok, ket });
	}
}

// ---------- Font: unduh woff2 latin dari Google Fonts, tanam sebagai data URI ----------
const UA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36";
async function fontCss() {
	const css = await (await fetch("https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;600;700;800&family=JetBrains+Mono:wght@400;600&display=swap", { headers: { "user-agent": UA } })).text();
	const faces = new Map(); // family|url -> weights
	for (const m of css.matchAll(/\/\* latin \*\/\s*@font-face\s*{([^}]*)}/g)) {
		const fam = m[1].match(/font-family:\s*'([^']+)'/)[1];
		const w = +m[1].match(/font-weight:\s*(\d+)/)[1];
		const url = m[1].match(/url\((https:[^)]+\.woff2)\)/)[1];
		const k = fam + "|" + url;
		faces.set(k, [...(faces.get(k) || []), w]);
	}
	if (faces.size < 2) throw new Error("font Google tidak terbaca");
	let out = "";
	for (const [k, ws] of faces) {
		const [fam, url] = k.split("|");
		const b64 = Buffer.from(await (await fetch(url)).arrayBuffer()).toString("base64");
		out += `@font-face{font-family:'${fam}';font-weight:${Math.min(...ws)} ${Math.max(...ws)};font-display:swap;src:url(data:font/woff2;base64,${b64}) format('woff2')}\n`;
	}
	return out;
}

// ---------- Ikon (gaya lucide, stroke 2) ----------
const I = {
	dash: '<rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/>',
	server: '<rect x="2" y="3" width="20" height="8" rx="2"/><rect x="2" y="13" width="20" height="8" rx="2"/><path d="M6 7h.01M6 17h.01"/>',
	box: '<path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/>',
	gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
	sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
	moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
	out: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5"/><path d="M21 12H9"/>',
	search: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>',
	plus: '<path d="M12 5v14M5 12h14"/>',
	chev: '<path d="m9 18 6-6-6-6"/>',
	okc: '<circle cx="12" cy="12" r="9"/><path d="m8.5 12 2.5 2.5 4.5-5"/>',
	x: '<path d="M18 6 6 18M6 6l12 12"/>',
	term: '<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>',
	pen: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
	stop: '<rect x="6" y="6" width="12" height="12" rx="1.5"/>'
};
const ic = (n, cls = "icon") => `<svg class="${cls}" viewBox="0 0 24 24" aria-hidden="true">${I[n]}</svg>`;
const badge = (kind, label) => `<span class="badge b-${kind}">${label}</span>`;

// ---------- Data (dari baseline mode mock) ----------
const SERVERS = [
	{ name: "prod-heavy-1", host: "ubuntu@10.0.0.11:22", cpu: [23.5, "4 cores"], ram: [45.2, "16000 MB"], disk: [67.0, "100 GB"],
		apps: [["api-core", "api.example.com", "8080"], ["web-shop", "shop.example.com", "3000"]] },
	{ name: "staging-1", host: "ubuntu@10.0.0.12:22", cpu: [23.5, "4 cores"], ram: [45.2, "16000 MB"], disk: [67.0, "100 GB"],
		apps: [["worker-mail", null, "9000"]] }
];

// ---------- Potongan layar ----------
const header = (dark) => `
<header class="top">
  <div class="brand"><span class="mark">D</span><b>Dockify</b></div>
  <div class="actions"><span class="ibtn" aria-label="Theme">${ic(dark ? "moon" : "sun")}</span><span class="ibtn" aria-label="Sign out">${ic("out")}</span></div>
</header>`;
const tabbar = (aktif) => `
<nav class="tabbar">${[["dash", "Dashboard"], ["server", "Servers"], ["box", "Apps"], ["gear", "Settings"]]
	.map(([i, l]) => `<span class="tab${l === aktif ? " on" : ""}"${l === aktif ? ' aria-current="page"' : ""}>${ic(i)}<span>${l}</span></span>`).join("")}</nav>`;

const meter = (label, [pct, cap]) => `<div class="meter"><div class="m-top"><small>${label}</small><b>${pct.toFixed(1)}%</b></div><div class="bar"><i style="width:${pct}%"></i></div><small class="m-cap">${cap}</small></div>`;

const dashboard = (dark) => `${header(dark)}
<main class="body">
  <div class="pagehead"><h1>Dashboard</h1><p class="sub">VM pool and app overview</p></div>
  <div class="kpis">
    <div class="kpi"><small>SERVERS</small><b>2</b><span>in pool</span></div>
    <div class="kpi"><small>ONLINE</small><b class="t-ok">2</b><span>of 2 servers</span></div>
    <div class="kpi"><small>APPS</small><b>3</b><span>on 2 servers</span></div>
    <div class="kpi"><small>RUNNING</small><b class="t-ok">3</b><span>of 3 apps</span></div>
  </div>
  <div class="sechead"><h2>Servers</h2><span class="link">View all</span></div>
  ${SERVERS.map((s) => `
  <section class="card srv">
    <div class="row-top"><div class="min0"><b class="name">${s.name}</b><code class="host">${s.host}</code></div>${badge("ok", "online")}</div>
    <div class="meters">${meter("CPU", s.cpu)}${meter("RAM", s.ram)}${meter("DISK", s.disk)}</div>
  </section>`).join("")}
</main>${tabbar("Dashboard")}`;

const apps = (dark) => `${header(dark)}
<main class="body">
  <div class="titlerow"><h1>Apps</h1><span class="btn btn-primary">${ic("plus")}Deploy App</span></div>
  <label class="input">${ic("search")}<span>Filter by name or domain</span></label>
  <div class="chips"><span class="chip on" aria-pressed="true">All</span><i class="vsep"></i><span class="chip">running</span><span class="chip">stopped</span><span class="chip">draft</span></div>
  <p class="meta">3 apps on 2 servers</p>
  ${SERVERS.map((s) => `
  <div class="group">
    <div class="grouphead"><b>${s.name}</b><code>${s.host.split("@")[1].replace(":22", "")}</code>${badge("ok", "online")}<span class="gcount">${s.apps.length} ${s.apps.length > 1 ? "apps" : "app"}</span></div>
    <section class="card list">${s.apps.map(([n, d, p]) => `
      <div class="approw">
        <div class="min0"><div class="ar-top"><b class="name">${n}</b>${badge("ok", "running")}</div>
        <code class="host">${d ? `${d}:${p}` : `internal :${p}`}</code>
        <div class="stats"><span>CPU <b>0.0%</b></span><span>MEM <b>-</b></span></div></div>
        ${ic("chev", "icon chev")}
      </div>`).join("")}
    </section>
  </div>`).join("")}
</main>${tabbar("Apps")}`;

const detail = (dark) => `${header(dark)}
<main class="body">
  <nav class="crumb"><span class="link">Apps</span><span>/</span><span>web-shop</span></nav>
  <div class="dtitle"><h1>web-shop</h1>${badge("ok", "running")}</div>
  <code class="host">shop.example.com:3000</code>
  <div class="alert a-ok" role="status">${ic("okc")}<div><b>Running on prod-heavy-1</b><span>Last deploy 2026-10-15 17:58 · success</span></div></div>
  <section class="card">
    <dl class="dgrid">
      <div><dt>SERVER</dt><dd>prod-heavy-1</dd></div><div><dt>PORT</dt><dd>3000</dd></div>
      <div><dt>DOMAIN</dt><dd>shop.example.com</dd></div><div><dt>INTERNAL</dt><dd class="mono">web-shop:3000</dd></div>
      <div><dt>IMAGE</dt><dd class="mono">nginx:alpine</dd></div><div><dt>RESTART</dt><dd>unless-stopped</dd></div>
    </dl>
    <div class="actgrid">
      <span class="btn btn-primary span2">${ic("pen")}Edit</span>
      <span class="btn btn-secondary">${ic("stop")}Stop</span>
      <span class="btn btn-ghost">${ic("term")}Console</span>
    </div>
  </section>
  <section class="card danger-zone">
    <h3>DANGER ZONE</h3>
    <p class="sub">Each action asks for confirmation first.</p>
    <div class="dgrid3"><span class="btn btn-dout">Redeploy</span><span class="btn btn-dout">Rollback</span><span class="btn btn-dout">Undeploy</span></div>
  </section>
</main>${tabbar("Apps")}`;

const confirmBody = `
    <h2 id="t">Undeploy web-shop?</h2>
    <p>This will stop and remove web-shop from prod-heavy-1. shop.example.com stops serving traffic until you deploy it again.</p>
    <div class="cgrid"><span class="btn btn-secondary">Cancel</span><span class="btn btn-destructive">Undeploy</span></div>`;
const sheet = (dark) => `${detail(dark)}
<div class="scrim"></div>
<div class="sheet" role="alertdialog" aria-labelledby="t"><i class="handle"></i>${confirmBody}</div>`;

const SCREENS = [["A", "Dashboard", dashboard], ["B", "Apps", apps], ["C", "App detail", detail], ["D", "Confirm Undeploy (HP)", sheet]];

// ---------- CSS ----------
const vars = (t) => Object.entries(t).map(([k, v]) => `--${k}:${v}`).join(";");
const CSS = `
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'Plus Jakarta Sans',system-ui,sans-serif;background:#e7e8ec;color:#17171c;-webkit-font-smoothing:antialiased;line-height:1.5}
code,.mono{font-family:'JetBrains Mono',ui-monospace,monospace}
.page{max-width:1640px;margin:0 auto;padding:24px 16px 64px}
.page>h1{font-size:26px;font-weight:800;letter-spacing:-.02em}
.lead{max-width:820px;color:#40414b;margin-top:6px}
.lead b{color:#17171c}
.opt{margin-top:44px;padding-top:24px;border-top:1px solid #cfd0d6}
.opt h2{font-size:22px;font-weight:800;letter-spacing:-.01em}
.opt>p{color:#40414b;margin-top:2px;max-width:820px}
.screens{display:flex;flex-wrap:wrap;gap:28px 40px;margin-top:16px}
.pair{display:flex;gap:14px;flex-wrap:wrap}
figure{flex:none}
figcaption{font-size:13px;font-weight:700;color:#40414b;margin-bottom:8px}
.legend{margin-top:20px;display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:4px 16px;font-size:12px;color:#40414b}
.legend h4{grid-column:1/-1;font-size:12px;font-weight:800;letter-spacing:.06em;margin-top:8px}
.legend span{display:flex;align-items:center;gap:8px;min-width:0}
.legend i{width:16px;height:16px;border-radius:4px;border:1px solid rgba(0,0,0,.15);flex:none}
.legend code{font-size:11.5px;color:#5a5b66}
.badges{grid-column:1/-1;display:flex;flex-wrap:wrap;gap:8px;padding:10px;border-radius:12px;margin-top:4px}
/* ===== Di dalam frame: HANYA token ===== */
.phone{width:360px;height:740px;border-radius:28px;overflow:hidden;border:7px solid #15161a;background:var(--bg);color:var(--ink);font-size:15px;line-height:1.5;display:flex;flex-direction:column;position:relative;box-shadow:0 10px 30px rgba(0,0,0,.16)}
.icon{width:18px;height:18px;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round;flex:none}
.top{display:flex;align-items:center;justify-content:space-between;padding:6px 8px 6px 14px;background:var(--surface);border-bottom:1px solid var(--line)}
.brand{display:flex;gap:10px;align-items:center}.brand b{font-size:16px;font-weight:800;letter-spacing:-.01em}
.mark{width:32px;height:32px;border-radius:10px;background:var(--accent-fill);color:var(--on-accent);display:grid;place-items:center;font-weight:800;font-size:15px}
.actions{display:flex;gap:2px}
.ibtn{width:44px;height:44px;border-radius:12px;display:grid;place-items:center;color:var(--ink2)}
.body{flex:1;overflow:hidden;padding:16px 16px 84px;display:grid;grid-template-columns:minmax(0,1fr);gap:12px;align-content:start}
h1{font-size:22px;font-weight:800;letter-spacing:-.02em;line-height:1.2}
.sub{font-size:12px;color:var(--muted)}
.meta{font-size:12px;color:var(--muted);margin-top:-4px}
.link{color:var(--accent);font-weight:600;font-size:13px}
.t-ok{color:var(--ok)}
.min0{min-width:0}
.kpis{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.kpi,.card{background:var(--surface);border:1px solid var(--line);border-radius:16px;box-shadow:var(--shadow)}
.kpi{padding:12px 14px}
.kpi small{display:block;font-size:11px;font-weight:800;letter-spacing:.06em;color:var(--muted)}
.kpi b{display:block;font-size:24px;font-weight:800;letter-spacing:-.02em;line-height:1.25}
.kpi span{font-size:12px;color:var(--muted)}
.sechead{display:flex;justify-content:space-between;align-items:baseline;margin-top:4px}
.sechead h2{font-size:16px;font-weight:800}
.card{padding:14px}
.row-top{display:flex;justify-content:space-between;gap:8px;align-items:flex-start}
.name{display:block;font-size:15px;font-weight:700;line-height:1.3}
.host{display:block;font-size:12px;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.badge{flex:none;display:inline-flex;align-items:center;font-size:10.5px;font-weight:800;letter-spacing:.06em;text-transform:uppercase;line-height:1;padding:4px 7px;border-radius:6px}
.b-ok{background:var(--ok-bg);color:var(--ok-text)}.b-warn{background:var(--warn-bg);color:var(--warn-text)}
.b-neutral{background:var(--sunk);color:var(--ink2);box-shadow:inset 0 0 0 1px var(--line)}.b-danger{background:var(--danger-bg);color:var(--danger-text)}
.meters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-top:12px}
.m-top{display:flex;justify-content:space-between;align-items:baseline}
.m-top small{font-size:11px;font-weight:800;letter-spacing:.06em;color:var(--muted)}
.m-top b{font-size:13px;font-weight:700;font-variant-numeric:tabular-nums}
.bar{height:6px;border-radius:99px;background:var(--chart-soft);overflow:hidden;margin:4px 0 2px}
.bar i{display:block;height:100%;background:var(--chart);border-radius:99px}
.m-cap{font-size:11px;color:var(--muted)}
.tabbar{position:absolute;left:0;right:0;bottom:0;height:64px;background:var(--surface);border-top:1px solid var(--line);display:grid;grid-template-columns:repeat(4,1fr)}
.tab{display:grid;place-items:center;align-content:center;gap:2px;font-size:11px;font-weight:600;color:var(--muted);position:relative}
.tab .icon{width:21px;height:21px}
.tab.on{color:var(--accent)}
.tab.on::before{content:"";position:absolute;top:0;left:26%;right:26%;height:3px;border-radius:0 0 3px 3px;background:var(--accent)}
.titlerow{display:flex;justify-content:space-between;align-items:center;gap:8px}
.btn{height:40px;border-radius:12px;padding:0 16px;display:flex;align-items:center;justify-content:center;gap:7px;font-weight:700;font-size:14px;white-space:nowrap}
.btn .icon{width:17px;height:17px}
.btn-primary{background:var(--accent-fill);color:var(--on-accent);box-shadow:var(--shadow)}
.btn-secondary{background:var(--surface);color:var(--ink);border:1px solid var(--control)}
.btn-ghost{color:var(--ink2)}
.btn-dout{color:var(--danger);border:1.5px solid var(--danger);padding:0 8px;background:transparent}
.btn-destructive{background:var(--danger-fill);color:#fff}
.input{height:40px;border-radius:12px;border:1px solid var(--control);background:var(--surface);display:flex;align-items:center;gap:8px;padding:0 12px;color:var(--muted);font-size:16px}
.chips{display:flex;gap:6px;align-items:center;margin:0 -16px;padding:0 16px;overflow:hidden}
.chip{flex:none;height:32px;padding:0 12px;border-radius:99px;border:1px solid var(--line-strong);display:flex;align-items:center;font-size:12.5px;font-weight:600;color:var(--ink2)}
.chip.on{background:var(--accent-soft);border-color:var(--accent);color:var(--accent)}
.vsep{width:1px;height:20px;background:var(--line-strong);flex:none;margin:0 2px}
.group{display:grid;gap:8px;margin-top:2px}
.grouphead{display:flex;align-items:center;gap:8px;font-size:13px;min-width:0}
.grouphead b{font-weight:800}
.grouphead code{font-size:12px;color:var(--muted)}
.gcount{margin-left:auto;font-size:12px;color:var(--muted)}
.list{padding:0 14px}
.approw{display:flex;align-items:center;gap:10px;padding:12px 0;border-bottom:1px solid var(--line)}
.approw:last-child{border-bottom:0}
.approw>.min0{flex:1}
.ar-top{display:flex;align-items:center;gap:8px}
.stats{display:flex;gap:14px;font-size:12px;color:var(--muted);margin-top:2px}
.stats b{color:var(--ink2);font-weight:600}
.chev{color:var(--muted)}
.crumb{display:flex;gap:6px;font-size:12px;color:var(--muted);align-items:baseline}.crumb .link{font-size:12px}
.dtitle{display:flex;align-items:center;gap:10px;margin-bottom:-10px}
.alert{display:flex;gap:10px;padding:12px;border-radius:12px;align-items:flex-start;font-size:13px}
.alert .icon{margin-top:1px}
.alert b{display:block;font-weight:700}
.a-ok{background:var(--ok-bg);color:var(--ok-text)}
.dgrid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px 14px}
.dgrid dt{font-size:11px;font-weight:800;letter-spacing:.06em;color:var(--muted)}
.dgrid dd{font-size:14px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.dgrid dd.mono{font-size:13px}
.actgrid{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:14px;padding-top:14px;border-top:1px solid var(--line)}
.span2{grid-column:span 2}
.danger-zone{box-shadow:none}
.danger-zone h3{font-size:12.8px;font-weight:800;letter-spacing:.06em;color:var(--danger)}
.dgrid3{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-top:10px}
.scrim{position:absolute;inset:0;background:rgba(3,6,15,.55)}
.sheet{position:absolute;left:0;right:0;bottom:0;background:var(--surface);border-radius:16px 16px 0 0;padding:10px 16px 24px;color:var(--ink)}
.handle{display:block;width:40px;height:4px;border-radius:99px;background:var(--line-strong);margin:0 auto 14px}
.sheet h2,.dialog h2{font-size:18px;font-weight:800;letter-spacing:-.01em}
.sheet p,.dialog p{font-size:14px;color:var(--ink2);margin-top:6px}
.cgrid{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:18px}
.desk{width:1000px;max-width:100%;height:520px;border-radius:14px;overflow:hidden;border:7px solid #15161a;position:relative;background:var(--bg);color:var(--ink)}
.desk .ghostui{padding:24px 40px;display:grid;gap:14px}
.desk .gline{height:14px;border-radius:6px;background:var(--line)}
.dialog{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);width:calc(100% - 48px);max-width:560px;background:var(--surface);border-radius:16px;padding:22px 24px 24px;box-shadow:0 20px 50px rgba(0,0,0,.3)}
.dialog .x{position:absolute;right:10px;top:10px;width:44px;height:44px;display:grid;place-items:center;color:var(--ink2);border-radius:12px}
.dialog .cgrid{max-width:320px;margin-left:auto}
`;

const frame = (o, mode, fn) => `<figure><figcaption>${mode === "light" ? "Terang" : "Gelap"}</figcaption>
<div class="phone" style="${vars(tok(o, mode))};color-scheme:${mode}">${fn(mode === "dark")}</div></figure>`;

const legend = (o) => {
	const row = (mode) => ORDER.filter((k) => k !== "shadow").map((k) => `<span><i style="background:${tok(o, mode)[k]}"></i>${k} <code>${tok(o, mode)[k]}</code></span>`).join("");
	const badges = (mode) => `<div class="badges" style="${vars(tok(o, mode))};background:var(--surface)">${badge("ok", "running")}${badge("ok", "online")}${badge("warn", "pending")}${badge("warn", "deploying")}${badge("neutral", "stopped")}${badge("neutral", "draft")}${badge("danger", "failed")}${badge("danger", "offline")}</div>`;
	return `<div class="legend"><h4>LENCANA STATUS</h4>${badges("light")}${badges("dark")}<h4>TOKEN TERANG</h4>${row("light")}<h4>TOKEN GELAP</h4>${row("dark")}</div>`;
};

const lolos = (id) => { const k = kontras.filter((x) => x.o === id); return `${k.filter((x) => x.ok).length}/${k.length}`; };

const desktop = (o) => `<section class="opt"><h2>Pola ResponsiveSheet di desktop (≥700px)</h2>
<p>Konfirmasi yang sama di layar lebar jadi popup tengah max-w 560 (contoh memakai opsi ${o.id}, terang). Tutup lewat X, backdrop, atau Escape. Di HP (&lt;700px) jadi panel bawah seperti layar D.</p>
<div class="screens"><div class="desk" style="${vars(tok(o, "light"))}">
  <div class="ghostui"><div class="gline" style="width:30%"></div><div class="gline" style="width:60%"></div><div class="gline" style="width:45%"></div><div class="gline" style="width:70%"></div></div>
  <div class="scrim"></div>
  <div class="dialog" role="alertdialog" aria-labelledby="t"><span class="x" aria-label="Close">${ic("x")}</span>${confirmBody}</div>
</div></div></section>`;

const html = (fonts) => `<!doctype html><html lang="id"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Dockify · 3 palet</title>
<style>${fonts}${CSS}</style></head><body><div class="page">
<h1>Dockify: tiga palet (Settings → Appearance)</h1>
<p class="lead">Semua kontras sudah dihitung (WCAG, terang dan gelap): teks ≥4.5:1, batas kontrol ≥3:1, <b>semua pasangan lolos</b> (${OPSI.map((o) => `opsi ${o.id}: ${lolos(o.id)}`).join(", ")}).</p>
<p class="lead">Struktur sama di ketiga opsi; yang berbeda hanya warna. Tiap layar: HP 360px, terang dan gelap berdampingan. Isi dari data mock Dockify (prod-heavy-1, staging-1, web-shop, api-core, worker-mail). Aksi berbahaya merah garis di zona bahaya, isian merah hanya di konfirmasi (K9). Label UI tetap bahasa Inggris.</p>
${OPSI.map((o) => `<section class="opt"><h2>${o.nama}</h2><p>${o.catatan}</p>
<div class="screens">${SCREENS.map(([k, l, fn]) => `<div><figcaption>${k}. ${l}</figcaption><div class="pair">${frame(o, "light", fn)}${frame(o, "dark", fn)}</div></div>`).join("")}</div>
${legend(o)}</section>`).join("")}
${desktop(OPSI[0])}
</div></body></html>`;

// ---------- kontras.md ----------
const md = () => {
	let s = `# Dockify S0: kontras token per opsi (WCAG 2.x)\n\nDihitung oleh \`gen.mjs\` (luminans relatif WCAG). Minimum: teks 4.5:1, batas kontrol dan grafis 3:1. Status: ${gagal ? `**${gagal} GAGAL**` : "semua lolos"}.\n\n`;
	s += `Ringkasan: ${OPSI.map((o) => `opsi ${o.id} ${lolos(o.id)} lolos`).join(" · ")} (12 pasangan wajib + 7 tambahan per tema).\n\n`;
	s += `Lencana Dockify: running/online = ok (\`ok-bg\` + \`ok-text\`); pending/initializing/deploying = warn (\`warn-bg\` + \`warn-text\`); stopped/draft = netral (\`sunk\` + \`ink2\`, garis \`line\`); error/failed/offline = danger (\`danger-bg\` + \`danger-text\`).\n\n`;
	for (const o of OPSI) {
		s += `## Opsi ${o.nama}\n\n| Pasangan | Min | Terang | Gelap | Catatan |\n| :--- | ---: | ---: | ---: | :--- |\n`;
		for (const [f, b, min, ket] of PAIRS) {
			const fx = f === "#ffffff" ? "white" : f;
			const cell = (mode) => { const x = kontras.find((k) => k.o === o.id && k.mode === mode && k.f === fx && k.b === b); return `${x.r.toFixed(2)}${x.ok ? "" : " **GAGAL**"}`; };
			s += `| ${fx} / ${b} | ${min} | ${cell("light")} | ${cell("dark")} | ${ket || ""} |\n`;
		}
		s += `\n| Token | Terang | Gelap |\n| :--- | :--- | :--- |\n`;
		for (const k of ORDER) s += `| ${k} | \`${tok(o, "light")[k]}\` | \`${tok(o, "dark")[k]}\` |\n`;
		s += `\n`;
	}
	return s;
};

const dir = new URL("./", import.meta.url);
writeFileSync(new URL("kontras.md", dir), md());
if (gagal) {
	for (const k of kontras.filter((x) => !x.ok)) console.error(`GAGAL opsi ${k.o} ${k.mode} ${k.f}/${k.b} ${k.r.toFixed(2)} < ${k.min}`);
	process.exit(1);
}
const out = html(await fontCss());
writeFileSync(new URL("preview-warna.html", dir), out);
console.log("ok", (out.length / 1024).toFixed(0) + " KB");
