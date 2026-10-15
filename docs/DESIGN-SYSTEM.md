# DESIGN-SYSTEM.md — Dockify

Sumber kebenaran tampilan Dockify. **UI baru atau perubahan UI WAJIB mengikuti dokumen ini.** Diturunkan dari
`~/workspace/tencent-dev-ops/standards/UI-STACK-DAN-DESIGN-SYSTEM.md` dan diadaptasi ke stack Go (`docs/UI-REVAMP-PLAN.md`, K1..K10).
Preview asal dan kontras terhitung: `docs/design/preview-warna.html`, `docs/design/kontras.md`, generator `docs/design/gen.mjs`.
Keputusan user 2026-10-15: ketiga palet dipakai dan bisa dipilih user; terang/gelap default mengikuti sistem.

## 1. Stack
Go `html/template` + htmx + CSS kustom di `internal/http/static/app.css` (ikut `go:embed`, disajikan `/static/app.css?v=<versi>`).
Tanpa framework CSS, tanpa Node saat build/runtime. Font: Plus Jakarta Sans 400/600/700/800 (UI) dan JetBrains Mono 400/600
(host, port, domain, log, kode) dari Google Fonts `display=swap`. Ikon: SVG inline stroke 2 lewat partial `ui_icon`.

## 2. Tema: palet × terang/gelap
- `<html data-palette="indigo|navy|teal" data-theme="light|dark">`. Keduanya **selalu** nilai efektif.
- Preferensi di `localStorage`: `dockify_palette` (`indigo` default) dan `dockify_theme` (`light|dark|system`, **default `system`**). Kunci lama `dockify-theme` dibaca sekali lalu dihapus.
- Skrip bootstrap **inline di `<head>`** (sebelum CSS) menulis kedua atribut + `<meta name="theme-color">`; selama `system`, ikuti `matchMedia('(prefers-color-scheme: dark)')` dan perubahannya. Tanpa `@media (prefers-color-scheme)` di CSS.
- CSS: lapis 2 didefinisikan per kombinasi: `:root[data-palette=indigo]`, `:root[data-palette=indigo][data-theme=dark]`, dst. (6 blok). Komponen hanya memakai token lapis 2 (`var(--ink)`, `var(--accent-fill)`...), tidak pernah hex.
- Kontrol: tombol ikon di header (terang → gelap → sistem → lawan dari tema efektif saat itu, supaya setiap klik terlihat berubah; ikon matahari/bulan/monitor, `aria-label` menyebut mode berikutnya) dan di **Settings → Appearance**: segmented "Palette" (Indigo / Navy / Teal) dan segmented "Theme" (Light / Dark / System). Halaman login ikut preferensi yang sama.

## 3. Token lapis 2 (nama tetap, nilai per palet)
Kegunaan tiap token sama dengan standar §3: `bg` latar halaman, `surface` kartu/header/sheet, `sunk` permukaan cekung (pill netral, wadah segmented),
`line`/`line-strong` pemisah/batas, `control` batas input/select (≥3:1 terhadap `surface`), `ink`/`ink2`/`muted` teks (muted hanya ≥12px),
`accent` teks/garis aksen, `accent-fill`+`on-accent` isian aksi utama, `accent-soft` hover/chip aktif, `brand2`/`brand2-soft` seri grafik kedua (RAM, disk, memori, jaringan) dari keluarga hue yang sama dengan aksen; `brand2-fill`+`on-brand2` disediakan tapi tidak dipakai tombol (keputusan user 2026-10-15: semua tombol aksi memakai `accent-fill`), `chart`/`chart-soft` bar dan grafik, `ok`/`warn`/`danger` + `-text` + `-bg` status, `danger-fill` isian tombol konfirmasi hapus, `shadow` satu-satunya bayangan.
Semua pasangan kontras sudah dihitung (38/38 lolos per palet, lihat `docs/design/kontras.md`). Jangan ubah nilai tanpa menghitung ulang.

### 3.1 Palet `indigo` (Indigo Dockify, default)
| Token | Terang | Gelap |
| :--- | :--- | :--- |
| bg | `#f5f7fb` | `#0b1020` |
| surface | `#ffffff` | `#151b2e` |
| sunk | `#f1f4f9` | `#0f1426` |
| line | `#e2e8f0` | `#262e48` |
| line-strong | `#cbd5e1` | `#36405f` |
| control | `#7a8699` | `#6e7a9c` |
| ink | `#0f172a` | `#e8ebf5` |
| ink2 | `#334155` | `#c0c7dc` |
| muted | `#5b677a` | `#929bb8` |
| accent | `#4f46e5` | `#a5b4fc` |
| accent-fill | `#4f46e5` | `#5850ec` |
| on-accent | `#ffffff` | `#ffffff` |
| accent-soft | `#eef0ff` | `#252a55` |
| brand2-fill | `#6d28d9` | `#6d28d9` |
| on-brand2 | `#ffffff` | `#ffffff` |
| brand2 | `#6d28d9` | `#c4b5fd` |
| brand2-soft | `#ede9fe` | `#2a1d55` |
| chart | `#6366f1` | `#818cf8` |
| chart-soft | `#e3e5fc` | `#252a55` |
| ok | `#15803d` | `#4ade80` |
| ok-text | `#166534` | `#86efac` |
| ok-bg | `#dcfce7` | `#10301e` |
| warn | `#b45309` | `#fbbf24` |
| warn-text | `#92400e` | `#fcd34d` |
| warn-bg | `#fef3c7` | `#33270a` |
| danger | `#c81e1e` | `#f87171` |
| danger-text | `#991b1b` | `#fca5a5` |
| danger-bg | `#fee2e2` | `#3b1618` |
| danger-fill | `#c81e1e` | `#c81e1e` |
| shadow | `0 1px 2px rgba(16,24,40,.06),0 1px 3px rgba(16,24,40,.05)` | `0 1px 2px rgba(0,0,0,.45)` |

### 3.2 Palet `navy` (Navy Brosur)
| Token | Terang | Gelap |
| :--- | :--- | :--- |
| bg | `#eef1f8` | `#0c0f1e` |
| surface | `#ffffff` | `#161a2f` |
| sunk | `#f5f7fc` | `#11142a` |
| line | `#e1e5f0` | `#272c48` |
| line-strong | `#c8cede` | `#363c5e` |
| control | `#7c83ad` | `#6b7399` |
| ink | `#141a33` | `#e9ebf7` |
| ink2 | `#3d4566` | `#bfc4de` |
| muted | `#656d8c` | `#8f95b4` |
| accent | `#2b3486` | `#a9b2ff` |
| accent-fill | `#2b3486` | `#4a54c9` |
| on-accent | `#ffffff` | `#ffffff` |
| accent-soft | `#e7e9f8` | `#242a5c` |
| brand2-fill | `#1e40af` | `#1e40af` |
| on-brand2 | `#ffffff` | `#ffffff` |
| brand2 | `#1e40af` | `#93c5fd` |
| brand2-soft | `#dbeafe` | `#1a2a55` |
| chart | `#4a54c9` | `#8c96ff` |
| chart-soft | `#e3e5fb` | `#242a5c` |
| ok | `#15803d` | `#4ade80` |
| ok-text | `#166534` | `#86efac` |
| ok-bg | `#dcfce7` | `#10301e` |
| warn | `#b45309` | `#fbbf24` |
| warn-text | `#92400e` | `#fcd34d` |
| warn-bg | `#fef3c7` | `#33270a` |
| danger | `#c81e1e` | `#f87171` |
| danger-text | `#991b1b` | `#fca5a5` |
| danger-bg | `#fee2e2` | `#3b1618` |
| danger-fill | `#c81e1e` | `#c81e1e` |
| shadow | `0 1px 2px rgba(16,24,40,.06),0 1px 3px rgba(16,24,40,.05)` | `0 1px 2px rgba(0,0,0,.45)` |

### 3.3 Palet `teal` (Slate Teal Ops)
| Token | Terang | Gelap |
| :--- | :--- | :--- |
| bg | `#f0f3f5` | `#0a1116` |
| surface | `#ffffff` | `#131c23` |
| sunk | `#f3f6f8` | `#0e161c` |
| line | `#dde3e8` | `#22303a` |
| line-strong | `#c5cfd7` | `#314350` |
| control | `#74828f` | `#687b89` |
| ink | `#0f1a22` | `#e6eef2` |
| ink2 | `#34434f` | `#b9c8d1` |
| muted | `#5b6975` | `#8a9ba7` |
| accent | `#0e6e78` | `#5fd4dd` |
| accent-fill | `#0e6e78` | `#0e6e78` |
| on-accent | `#ffffff` | `#ffffff` |
| accent-soft | `#e0f1f2` | `#103338` |
| brand2-fill | `#075985` | `#075985` |
| on-brand2 | `#ffffff` | `#ffffff` |
| brand2 | `#075985` | `#7dd3fc` |
| brand2-soft | `#e0f2fe` | `#0f2a3a` |
| chart | `#0891b2` | `#38bdf8` |
| chart-soft | `#dcf0f6` | `#0f2c3a` |
| ok | `#15803d` | `#4ade80` |
| ok-text | `#166534` | `#86efac` |
| ok-bg | `#dcfce7` | `#10301e` |
| warn | `#b45309` | `#fbbf24` |
| warn-text | `#92400e` | `#fcd34d` |
| warn-bg | `#fef3c7` | `#33270a` |
| danger | `#c81e1e` | `#f87171` |
| danger-text | `#991b1b` | `#fca5a5` |
| danger-bg | `#fee2e2` | `#3b1618` |
| danger-fill | `#c81e1e` | `#c81e1e` |
| shadow | `0 1px 2px rgba(16,24,40,.06),0 1px 3px rgba(16,24,40,.05)` | `0 1px 2px rgba(0,0,0,.45)` |

### 3.4 Lencana status
| Status Dockify | Token |
| :--- | :--- |
| running, online, success | `ok-bg` + `ok-text` (+ titik berdenyut untuk running/online) |
| pending, initializing, deploying, created | `warn-bg` + `warn-text` |
| stopped, draft | `sunk` + `ink2`, garis `line` |
| error, failed, offline | `danger-bg` + `danger-text` |
Lencana: 10 sampai 11px, 800, huruf kapital, radius 6, font mono untuk nilai teknis.

## 4. Tipografi, spasi, radius
Body 15px/1.5. KPI 24px/800 tracking -0.02em. Angka sekunder 20px/800. Judul kartu 15px/700. Label 12.8px. Meta/chip 11 sampai 12px (chip 600).
Label tab bawah 11px. Input/select **16px**. Skala spasi 4px: padding halaman 14 sampai 16px, antar kartu 12px, antar kontrol 8px.
Radius: kartu 16, kontrol/tombol 12, lencana 6, chip/pill penuh. Target sentuh aksi utama dan toggle ≥44px; chip 32px dengan jarak ≥6px.

## 5. Komponen (partial bersama, varian di partial bukan di pemakaian)
| Kebutuhan | Partial / kelas | Catatan |
| :--- | :--- | :--- |
| Tombol | `.btn` + varian `primary` `secondary` `ghost` `destructive` `destructive-outline` `link`; ukuran 44 (default) / 36 (`sm`) / 48 (`lg`); ikon 44/40 | satu `primary` per layar/panel; dalam satu kelompok varian harus berbeda |
| Input + label | `.field` (label 12.8px/600 + input 16px, border `control`, fokus outline 3px `accent`) | tanpa titik dua di label |
| Select | `<select class="select">` native digayakan + chevron SVG | penyimpangan K4 yang disetujui |
| Dialog / alert-dialog / drawer | `ui_sheet`: `<dialog class="sheet">` + `openSheet(id)`/`closeSheet(id)`; ≥700px popup tengah max-w 560 radius 16 max-h 86dvh; <700px panel bawah max-w 560 radius atas 16 handle 40×4 max-h 92dvh + safe-area; animasi ≤240 ms; hormati `prefers-reduced-motion`; tutup via X, backdrop, Escape; **tanpa tautan "Tutup" di bawah** | menggantikan semua `confirm()` dan modal inline |
| Kartu | `.card` = `surface` + border `line` + `shadow`, radius 16 | bayangan hanya di sini |
| Lencana | `.badge .badge-<status>` | §3.4 |
| Segmented | `ui_segmented`: wadah `sunk` radius 12, segmen aktif `surface` + bayangan, `aria-pressed` | rentang grafik, Simple/Advanced, Palette/Theme |
| Chip filter | `.chips` satu baris scroll horizontal tanpa scrollbar, pudar tepi kanan; chip "All" di depan; aktif = `accent-soft` + teks `accent`; meta hasil + "Reset filter" di bawah | tidak pernah isian penuh |
| Progress | `.bar` (`chart` di atas `chart-soft`) | gauge CPU/RAM/Disk |
| Alert / toast | `.alert-ok|warn|danger` (`role=alert` galat); toast `role=status` 4 dtk untuk flash | |
| Pemisah | `.sep` | |
| Ikon | `ui_icon` nama: sun moon monitor logout menu x check alert refresh plus trash external chevron | SVG inline stroke 2 |

### 5.1 Hierarki aksi (K9)
- **Primary** (`accent-fill`): satu per layar: Edit (app/server running), Start (stopped), Deploy (draft), Save, Sign in, Deploy App (daftar Apps), Add Server (daftar Servers). Tidak ada warna tombol kedua; semua palet memakai keluarga hue aksennya sendiri.
- **Secondary**: Stop, Cancel, Save as Draft, tombol ikon header.
- **Ghost**: Console, Tail 50/200/500, Copy, Show.
- **Aksi berbahaya = merah**: Redeploy, Rollback, Init Worker, Prune Disk, Undeploy, Remove Server, Delete Draft, Clear History, Roll Secret, Update Dockify, Redeploy All. Pemicu = `destructive-outline` di **zona bahaya** terpisah (judul "Danger zone", garis atas); konfirmasi selalu lewat alert-dialog dengan kalimat akibat; tombol konfirmasi = `destructive` (isian `danger-fill`, teks putih).
- **Status tercapai** (online, running, sudah init) = tint `ok-bg`/`ok-text` + ikon centang, bukan tombol.

## 6. Layout
- Mobile first 360×740, uji 390×844 dan 1280×800. Tanpa overflow horizontal halaman (hanya baris chip).
- Shell: header ringkas (logo + nama + tombol ikon 44px: tema, keluar) + **tab bar bawah** <900px (Dashboard, Servers, Apps, Settings; ikon + label 11px, garis aktif, `aria-current`, `env(safe-area-inset-bottom)`). ≥900px: tab pindah ke bawah header, konten max 1100px, kartu grid 2 kolom. About pindah ke Settings.
- Daftar: <700px baris kartu (grid `minmax(0,1.3fr) 1fr 1fr 44px`, `white-space:nowrap`), ≥700px tabel. KPI grid 2×2 di HP, 4 kolom di desktop.
- Panel detail: header (nama 18/800 + meta 12.8 muted + X) → banner status → `detail-grid` 2 kolom (dt 11/800 kapital muted, dd 14/600) → seksi berjudul (12.8/800 kapital, garis atas) → aksi (grid 2 kolom, primary penuh lebar) → zona bahaya.

## 7. Performa dan aksesibilitas (diukur, `tests/e2e/ui.mjs`)
Viewport 390, CPU 4×, latensi 80 ms, median 5 ulangan: navigasi antar halaman utama ≤300 ms (localhost). DOM ≤12 node per baris daftar, <2.500 node untuk 200 baris.
Garis progres 3px saat navigasi >150 ms (htmx `hx-boost` opsional). Fokus terlihat 3px `accent`; Escape menutup sheet; `aria-pressed`; `aria-labelledby` di dialog.
Bahasa UI Inggris, label tanpa titik dua, tombol kata kerja spesifik, tanpa emoji.

## 8. Checklist PR UI
- [ ] Semua komponen lewat partial/kelas bersama; elemen native tersisa hanya `select` (K4) dan `dialog`
- [ ] Tanpa hex di template dan CSS komponen (kecuali tema xterm); tanpa `style=""` inline baru
- [ ] 3 palet × terang/gelap dicek; kontras tidak diubah tanpa hitung ulang
- [ ] Tanpa overflow horizontal di 360 dan 390; ≥900 rapi
- [ ] Hierarki §5.1: satu primary; aksi berbahaya merah outline + alert-dialog; status = tint
- [ ] Sheet responsif: popup tengah di desktop, panel bawah di HP
- [ ] Target sentuh, fokus, Escape
- [ ] Angka perf §7 dilaporkan (sebelum/sesudah) bila menyentuh daftar/navigasi
- [ ] `go build ./... && go vet ./... && go test ./...` hijau; screenshot 360/390/1280 × terang/gelap terlampir
