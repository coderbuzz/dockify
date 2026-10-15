# Rencana revamp UI/UX Dockify (design system layout Dashboard di stack Go)

Status: **disetujui user 2026-10-15** (K1 stack dipertahankan, S0 dimulai, staging via tunnel). Leader: Fable 5.1 (akun Max). Sumber aturan:
`~/workspace/tencent-dev-ops/standards/UI-STACK-DAN-DESIGN-SYSTEM.md` (standar UI) dan
`WORKFLOW-PLANNER-EKSEKUTOR.md` (alur kerja). Setelah user memilih opsi warna, `docs/DESIGN-SYSTEM.md` di repo ini jadi rujukan utama.

## 1. Kesimpulan kelayakan

**Bisa, tanpa migrasi stack.** Standar UI terdiri dari lima lapis; hanya satu yang terikat ke SvelteKit.

| Lapis standar | Terikat framework? | Di Go `html/template` + htmx |
| :--- | :--- | :--- |
| Token warna dua lapis, mode gelap (§3, §4) | Tidak | CSS custom properties, sudah ada polanya di `layout.html` |
| Anatomi dan varian komponen (§5, §7.1, §7.3) | Hanya nama paketnya (shadcn-svelte) | Partial `{{define}}` + kelas CSS bersama + sedikit JS vanilla |
| Hierarki, layout mobile first, tipografi (§6, §7, §8) | Tidak | CSS |
| Performa terukur, aksesibilitas (§9, §10) | Tidak | Playwright dari VM, server-rendered jadi lebih ringan |
| Checklist merge (§11) | Tidak | Sama, butir "shadcn" diganti "partial bersama" |

Standar versi 2026-10-15 juga menyebut eksplisit: repo dengan stack lain **tidak dimigrasi tanpa keputusan user**, dan
semua ketentuan selain "komponen dari shadcn" tetap berlaku.

### Opsi yang dipertimbangkan

| Opsi | Isi | Penilaian |
| :--- | :--- | :--- |
| **A. Pertahankan stack, terapkan design system** (rekomendasi) | CSS kustom ditulis ulang sebagai `app.css` ber-token, komponen jadi partial bersama, `<dialog>` native untuk sheet/konfirmasi, htmx tetap | Satu biner tetap, tanpa toolchain Node di build/CI, diff terkecil, semua aturan terpenuhi |
| B. Tailwind v4 CLI standalone + kelas gaya shadcn | Tambah langkah build CSS (biner tailwind) di `release.sh` dan CI, kelas utilitas di template | Memenuhi aturan tapi menambah toolchain dan membuat template lebih panjang; tidak ada manfaat nyata untuk ±20 template |
| C. `templ` (komponen Go ter-generate) | Ganti `html/template` dengan `templ` | Migrasi besar tanpa dampak ke UI; tidak relevan dengan revamp |
| D. SvelteKit terpisah + shadcn-svelte | Frontend SPA di-embed atau dihosting terpisah | Bertentangan dengan filosofi "single Go binary"; dua stack, dua build, auth ganda |

## 2. Kondisi saat ini (baseline, mode mock, 2026-10-15)

Build `go build ./...`, `go vet`, `go test ./...` hijau (Go 1.26.0). Screenshot baseline di `docs/ui-revamp/baseline/`.

| Hal | Temuan | Aturan yang dilanggar |
| :--- | :--- | :--- |
| CSS | 759 baris di satu `<style>` di `layout.html`; `login.html` punya `<style>` sendiri dengan 21 hex | §3 token semantik, satu sumber |
| Token | Satu lapis (`--bg`, `--accent`, `--green`...), tanpa `control`, `ink2`, `-text` untuk status | §3 dua lapis |
| Inline style | 243 `style="..."` di 20 template | §5 komponen bersama |
| Mode gelap | `html.dark`, skrip toggle di **akhir body** (FOUC saat tema gelap), tanpa opsi `system` eksplisit | §4 skrip inline di `<head>` |
| Konfirmasi | 8 `confirm()` native; 4 modal kustom dengan inline style, tanpa Escape/fokus | §5 alert-dialog, §7.3 ResponsiveSheet |
| Tombol | Redeploy, Rollback, Init Worker isian merah penuh tanpa konfirmasi (Redeploy, Init Worker); Edit isian biru; 6 tombol sejajar sama bobot di detail app | §7.1 satu primary per layar; K9 pemicu berbahaya = outline merah + alert-dialog, isian merah hanya di konfirmasi |
| Tombol di HP | `btn-group` jadi kolom penuh lebar, semua sama rata | §7.1 |
| Navigasi HP | Hamburger + menu dropdown | §8 tab bar bawah |
| Tabel di HP | Kolom terpotong, scroll horizontal di dalam `.table-wrap` | §8 tanpa overflow (hanya baris chip) |
| KPI di HP | 4 kartu bertumpuk penuh lebar | §8 grid 2 kolom |
| Tipografi | Body 14px, radius kartu 8px, lencana monospace huruf kecil | §6 body 15, radius 16/12/6, lencana 10-11px 800 kapital |
| Select | 6 `<select>` native tanpa gaya | §5 (lihat penyimpangan K4) |
| DOM | 86 sampai 250 node per halaman dengan 2 server + 3 app; ±20 node per baris tabel | §9 target <2.500 untuk 200 baris: terpenuhi sampai ±120 baris, perlu baris lebih ringan |
| Performa | Semua halaman server-rendered; pindah halaman = navigasi penuh (bukan pindah tab SPA) | §9 diukur sebagai waktu navigasi, target ≤300 ms di CPU 4x (localhost) |

## 3. Keputusan (K1..K8)

- **K1 Stack dipertahankan**: Go `html/template` + htmx + CSS kustom; tanpa Node di build/runtime. Disetujui user 2026-10-15.
- **K2 CSS keluar dari `layout.html`** ke `internal/http/static/app.css` (ikut `go:embed`, disajikan `/static/app.css?v=<versi>` dengan cache lama). Token lapis 1 (palet) dan lapis 2 (semantik, nama persis seperti standar §3) di `:root` dan `:root[data-theme=dark]`.
- **K3 Komponen = partial bersama** di `internal/http/templates/ui_*.html`: `ui_button` (varian primary/brand/secondary/ghost/destructive/destructive-outline/link; ukuran 44/36/48/ikon), `ui_badge`, `ui_card`, `ui_alert`, `ui_segmented`, `ui_sheet` (dialog/drawer responsif), `ui_icon` (SVG inline stroke 2). Varian diubah di partial, bukan di pemakaian. Inline style ditiadakan.
- **K4 `<select>` native tetap dipakai, digayakan** (border `control`, chevron SVG, 16px). Alasan: 6 select enum sederhana; listbox kustom vanilla JS menambah kode dan risiko aksesibilitas tanpa manfaat. Dicatat sebagai penyimpangan §5 yang disetujui.
- **K5 `<dialog>` native sebagai ResponsiveSheet**: `showModal()` memberi Escape, fokus terkurung, dan backdrop gratis. CSS: ≥700px popup tengah max-w 560 radius 16; <700px panel bawah radius atas 16 + handle 40×4 + safe-area; animasi ≤240 ms, hormati `prefers-reduced-motion`. Semua `confirm()` dan modal inline diganti.
- **K6 Mode gelap**: pindah ke `data-theme` (`light|dark`) + preferensi `localStorage.dockify_theme` (`light|dark|system`, default `system`), skrip inline di `<head>`, satu blok `:root[data-theme=dark]`, tanpa `@media (prefers-color-scheme)` di CSS. Kunci lama `dockify-theme` dibaca sekali lalu dimigrasi.
- **K7 Shell**: header ringkas (logo + tombol ikon 44px tema/keluar) + **tab bar bawah** di <900px (Dashboard, Servers, Apps, Settings; About pindah ke Settings), tab di bawah header mulai ≥900px, konten max 1100px. Tabel jadi daftar kartu baris di <700px (grid kolom tetap `minmax(0,1.3fr) 1fr 1fr 44px`), tabel penuh di ≥700px.
- **K9 Aksi berbahaya = merah** (keputusan user 2026-10-15): Redeploy, Rollback, Init Worker, Prune Disk, Undeploy, Remove Server, Delete Draft, Clear History, Roll Secret, Update Dockify. Pemicu di bar aksi memakai `destructive-outline` (garis + teks `danger`), tombol konfirmasi akhir di alert-dialog memakai `destructive` (isian `danger-fill`). Satu primary per layar tetap berlaku untuk aksi tidak berbahaya (Edit, Start, Deploy draft, Simpan). Konfirmasi selalu lewat alert-dialog dengan kalimat akibatnya.
- **K8 Pengukuran**: skrip `tests/e2e/ui.mjs` (Playwright dari `~/.local/share/pw-tools`): screenshot 360x740, 390x844, 1280x800 × terang/gelap; cek overflow horizontal; hitung node DOM; waktu navigasi antar halaman utama median 5 ulangan dengan CPU 4x dan latensi 80 ms; cek kontras token (≥4.5:1 teks, ≥3:1 kontrol) di kedua tema. Hasil "sebelum/sesudah" dilampirkan di tiap PR UI.

Di luar cakupan: fitur baru, perubahan API/JSON, perubahan skema DB, terminal xterm (tema xterm tetap hex, dikecualikan dari grep hex), grafik SVG (hanya warnanya diganti token `chart`).

## 4. Pemetaan komponen shadcn → Go

| Kebutuhan (§5) | Di Dockify | Bentuk |
| :--- | :--- | :--- |
| button | `{{template "ui_button" (dict ...)}}` atau kelas `btn btn-primary` dst. | kelas CSS; partial untuk tombol ikon |
| input, label | `<label class="field">Nama<input ...></label>` | kelas; font 16px |
| select | `<select class="select">` + chevron | kelas (K4) |
| dialog, alert-dialog | `<dialog class="sheet">` + `ui_sheet` | partial + 1 fungsi JS `openSheet(id)` |
| drawer | sama dengan dialog di <700px | CSS |
| card | `.card` dengan `line` + `shadow` | kelas |
| badge | `.badge .badge-{status}` 10-11px 800 kapital, tint `-bg` + `-text` | kelas |
| toggle-group segmented | rentang grafik Realtime/1h/6h/24h/7d, mode compose Simple/Advanced | `ui_segmented`: wadah `sunk` radius 12, segmen aktif `surface` + bayangan, `aria-pressed` |
| progress | gauge CPU/RAM/Disk yang ada | kelas, warna token |
| alert, sonner | banner status di panel; flash setelah aksi → toast `role=status` 4 dtk | kelas + 1 fungsi JS |
| separator | `.sep` | kelas |
| tabs | rentang grafik via htmx `hx-get` (sudah ada); navigasi = `<a aria-current>` | tetap |
| Icon | `ui_icon` dengan `{{if eq .name "x"}}...` | partial SVG |

## 5. Langkah ber-ID (S0..S8)

Tiap langkah: cabang `feat/ui-s<N>-<topik>` dari `main` (konvensi `AGENTS.md` repo), `go build ./... && go vet ./... && go test ./...` hijau,
PR dengan bukti (screenshot 3 viewport × 2 tema untuk PR UI, tabel perf sebelum/sesudah), squash merge oleh leader.

| ID | Langkah | Kriteria selesai | Model | Bukti |
| :--- | :--- | :--- | :--- | :--- |
| S0 | Preview statis 3 opsi warna (HTML tanpa JS, font data URI): layar Dashboard, Apps, App detail, sheet konfirmasi, HP 360 terang+gelap. Opsi: (1) Indigo Dockify (brand sekarang dirapikan), (2) Navy Brosur (palet standar), (3) Slate + Teal "ops". Lalu tulis `docs/DESIGN-SYSTEM.md` dari opsi terpilih dan tambah aturan di `AGENTS.md` | User memilih; DESIGN-SYSTEM.md berisi nilai token kedua tema dengan kontras terhitung | Opus (preview/token, high), leader finalisasi | tautan `share`, tabel kontras |
| S1 | `app.css` ter-embed + token dua lapis + `data-theme` + skrip head (K2, K6); `login.html` memakai CSS bersama; `layout.html` tinggal struktur | Tampilan setara sebelumnya kecuali warna token; grep hex di template = hanya xterm; tanpa FOUC gelap | Sonnet (medium) | screenshot, grep |
| S2 | Partial komponen (K3, K5): button varian, badge, card, alert, segmented, sheet, icon, toast; ganti 8 `confirm()` + 4 modal inline | Tidak ada `confirm(`; semua tombol pakai varian; Escape/backdrop/fokus jalan | Sonnet; review Opus (UI baru) | screenshot sheet 390 + 1280 |
| S3 | Shell (K7): header, tab bar bawah, tab atas ≥900, container, About → Settings | 360/390 tanpa overflow; `aria-current`; safe-area | Sonnet | screenshot 3 viewport |
| S4 | Dashboard + daftar Servers/Apps: KPI 2x2 di HP, baris kartu <700px, tabel ≥700px, filter satu baris + meta hasil + "Reset filter", urut tetap | DOM ≤12 node per baris; tanpa overflow; filter/urut masih jalan | Sonnet | screenshot, node count |
| S5 | Detail App dan Server: bar aksi berhierarki (satu primary: Edit/Start/Deploy; aksi berbahaya K9 merah `destructive-outline` + alert-dialog; Console/Logs ghost), `detail-grid`, banner status, rentang grafik segmented | Sesuai §7.1, §7.2, K9; tabel inline style = 0 | Sonnet; review Opus | screenshot |
| S6 | Form (apps add/edit, servers add/edit, settings, import/export): label 12.8px, input 16px, select bergaya, modal → sheet, baris env/file/domain memakai grid kolom tetap | Inline style = 0 di template form; iOS tidak zoom | Sonnet | screenshot 390 |
| S7 | `tests/e2e/ui.mjs` (K8) + perbaikan temuan perf/a11y/kontras | Semua target §9/§10 terpenuhi; tabel sebelum/sesudah | Haiku jalankan (low), Sonnet perbaiki | tabel perf, laporan kontras |
| S8 | Audit akhir leader (worktree `../dockify-audit`), perbarui `SPEC.md` §UI Style Guide → rujuk `docs/DESIGN-SYSTEM.md`, `README` screenshot, release patch | Checklist §11 semua tercentang; `./scripts/release.sh` | Fable | temuan bernomor, tag rilis |

Perkiraan: 8 PR + 1 PR dokumen ini. Eksekutor: satu sesi Sonnet (`dockify · eksekusi ui`), effort medium, dibuat setelah S0 selesai; ditutup setelah S7 di-merge.

## 6. Staging dan verifikasi

Dockify bukan Cloudflare Worker. "Staging" = biner hasil PR dijalankan di VM ini dengan `DOCKIFY_DEV_MOCK=true` di port lokal,
diverifikasi Playwright (screenshot + perf). Agar user bisa mencoba dari HP, perlu hostname tunnel baru di belakang Cloudflare Access
(mis. `dockify-staging.coderbuzz.dev`, seperti `zh.coderbuzz.dev`); itu perubahan DNS/tunnel, **butuh persetujuan**. Tanpa itu, user mencoba
lewat `air` di mesin sendiri atau cukup dari screenshot.

## 7. Aturan untuk eksekutor

- Baca `AGENTS.md` repo, `docs/DESIGN-SYSTEM.md`, dan dokumen ini sampai habis. Ikuti K1..K8 apa adanya; penyimpangan dijelaskan di PR.
- Hanya token semantik di CSS komponen; tanpa hex di template (kecuali tema xterm). Tanpa inline `style=` baru.
- Jangan sentuh handler Go kecuali untuk data yang memang belum ada di template (mis. flash → toast). Jangan ubah API JSON.
- Setiap PR: build/vet/test hijau, screenshot, angka perf bila menyentuh daftar/navigasi. `send_message` ke leader dengan ID langkah, URL PR, bukti.
- Jangan merge, push `main`, force-push, deploy, atau menyentuh secret.
