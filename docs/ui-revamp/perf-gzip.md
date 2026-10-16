# Perf gzip (respons HTTP), cabang `feat/gzip`

`chi middleware.Compress` level 5 (`internal/http/compress.go`) untuk text/html, text/css, text/javascript,
application/javascript, application/json, image/svg+xml, application/manifest+json. Badan < 1 KB dikirim apa adanya
(`minSizeWriter`), PNG tidak dikompres, WebSocket (101 tanpa Content-Type, di-hijack) lewat tanpa diubah.
`Cache-Control` tetap: `sw.js` `no-cache`, aset lain `public, max-age=604800` (diuji di `compress_test.go`).
Aset statis tidak punya ETag (embed FS tanpa modtime); perilaku itu tidak berubah.

Build mock, port 18090, auth aktif, konteks baru (cache dingin). Sebelum = `origin/main` d7fa1d8 (sama dengan `perf-s9.md`).

## Transfer dingin, laporan `tests/e2e/ui.mjs` (total di kawat, termasuk Google Fonts)

| Halaman | KB sebelum | KB sesudah | Turun |
| --- | --- | --- | --- |
| dashboard | 209 | 105 | 50% |
| servers | 199 | 104 | 48% |
| apps | 205 | 105 | 49% |
| app-detail | 209 | 107 | 49% |

## Dipisah per asal (body + header, skrip Playwright yang sama polanya)

| Halaman | Dockify sebelum → sesudah | Google Fonts (tidak berubah) |
| --- | --- | --- |
| dashboard | 136,2 → 44,5 KB (−67%) | 28,5 KB |
| servers | 134,9 → 44,3 KB (−67%) | 28,5 KB |
| apps | 136,0 → 44,6 KB (−67%) | 28,5 KB |
| app-detail | 135,0 → 44,4 KB (−67%) | 28,5 KB |

Target ≥50% tercapai untuk semua byte yang dilayani Dockify (−67%). Total halaman turun 48–50% karena sisanya font
woff2 dari fonts.gstatic.com (sudah terkompresi, bukan dari server ini).

Navigasi tidak berubah (median DCL dashboard/servers/apps/app-detail 192/178/191/206 ms vs 192/178/186/199 ms; selisih
dalam derau run). ui.mjs PASS.
