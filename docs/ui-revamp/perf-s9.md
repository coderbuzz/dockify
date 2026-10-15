# Perf S9 (hx-boost + preload + head-support), cabang `feat/ui-s9-boost`

`UI_USER=admin UI_PASS=uji123 node tests/e2e/ui.mjs http://127.0.0.1:18090 <out>` (build mock, viewport 390, CPU 4x, latensi 80 ms, median 5).
Sebelum = `perf-sesudah.md` (S7, navigasi dokumen penuh); sesudah = run ini.

## Navigasi antar halaman utama

| Kasus | Sebelum (S7) | Sesudah (S9) | Target |
| --- | --- | --- | --- |
| Klik tab Servers → Apps, boost, preload (touchstart dulu) | - | **65 ms** (`htmx:beforeRequest` → `afterSettle`) | ≤150 ms ✅ |
| Sama, tanpa preload | - | **153 ms** | ≤250 ms ✅ |
| `goto` penuh dashboard / servers / apps / app-detail (DCL) | 300 / 301 / 301 / 328 ms | 190 / 180 / 190 / 197 ms | (bukan target; sama persis halaman penuh) |

Header tidak dirender ulang (`isConnected` true), `aria-current` dan `document.title` mengikuti halaman, scroll ke atas, Back/Forward bekerja.
Kebocoran: 10 bolak-balik daftar ↔ detail app (grafik live + WebSocket): node DOM 236 → 236, daftar `dockifyCleanups` kosong di halaman daftar.

## Transfer dingin (di kawat)

| Halaman | KB S7 → S9 | Request S7 → S9 |
| --- | --- | --- |
| dashboard | 126 → 207 | 7 → 12 |
| servers | 120 → 196 | 7 → 12 |
| apps | 122 → 203 | 7 → 12 |
| app-detail | 192 → 206 | 10 → 13 |

Kenaikan cold-load (+~80 KB, +5 request) berasal dari htmx 2.0.4 + 2 ekstensi + charts.js + console.js yang kini dimuat di semua halaman, plus manifest/ikon.
Sekali dimuat, service worker meng-cache `/static/*` (cache-first); kompresi gzip HTTP tetap tugas terpisah yang sudah dicatat.
