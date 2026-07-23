# TODO

## 1. Debug Disk Usage 0 di App Page

### Masalah
Disk usage menampilkan `0 B` padahal beberapa app seperti `tools.amg.id` punya disk usage besar.

### Dugaan
- v0.5.10 mengubah `UpdateDiskUsage` ke subquery `MAX(created_at)` — sudah benar
- Tapi `collectAppDiskUsage` berjalan tiap **5 menit** — mungkin belum pernah fire
- Atau command `du -sb /opt/dockify/apps/app-<id>` gagal (path salah, permission, dll)

### Langkah
1. **Remote ke worker** — pakai private key yang ada di chat (copy paste ulang, file `/tmp/dockify_worker_key` terhapus waktu cleanup kemarin)
2. **Test `du -sb` langsung**:
   ```bash
   du -sb /opt/dockify/apps/app-7 2>/dev/null
   ```
   (app-7 = tools.amg.id dari label compose project)
3. **Cek DB** — apakah `disk_usage_bytes` sudah terisi:
   ```bash
   sqlite3 /opt/dockify/data/dockify.db \
     "SELECT app_id, disk_usage_bytes, created_at FROM container_stats WHERE disk_usage_bytes > 0 LIMIT 5;"
   ```
4. **Cek row terbaru untuk app 7**:
   ```bash
   sqlite3 /opt/dockify/data/dockify.db \
     "SELECT app_id, disk_usage_bytes, created_at FROM container_stats WHERE app_id = 7 ORDER BY created_at DESC LIMIT 5;"
   ```
5. **Cek log server** — apakah ada error dari `collectDiskUsageForApps`:
   ```bash
   journalctl -u dockify --since "1 hour ago" | grep -i "disk\|collect"
   ```
6. **Jika perlu, paksa collect manual**:
   - SSH ke worker
   - Test `du -sb` untuk semua app directory
   - Update langsung via sqlite

### Fix jika terbukti bug
- Jika `du` command gagal: perbaiki path atau permission
- Jika subquery tidak match: ganti pendekatan (update ALL rows atau INSERT terpisah)
- Jika 5 menit terlalu lama: turunkan interval atau trigger saat page load

---

## 2. Caddy Per-Domain Traffic Metrics (Ditunda)

### Masalah
Server-level `"metrics":{}` menghasilkan `caddy_http_requests_total` tanpa label `host`, sehingga `collectCaddyTraffic` tidak bisa breakdown per-domain.

### Opsi yang perlu di-test (local Orbstack)
1. **`subroute` handler wrapping** — taruh `metrics` handler di dalam `subroute` bersama `reverse_proxy`, lihat apakah berfungsi sebagai middleware pass-through
2. **Custom Caddy build** dengan `xcaddy` jika `subroute` juga gagal

### Status
**Ditunda** — user setuju untuk skip dulu.

---

## 3. Verifikasi LIVE Indicator + Range Switch

### Status
v0.5.10 tambah show indicator di `ws.onmessage` — harusnya survive htmx swap.

### Verifikasi
- Buka app page → ● LIVE muncul
- Klik 1h/6h/24h/7d → ● LIVE tetap visible
- Server page → sama

---

## 4. Verifikasi UpdateDiskUsage Subquery

### Status
v0.5.10 ganti `WHERE app_id=? AND created_at=?` jadi `WHERE app_id=? AND created_at=(SELECT MAX(created_at)...)`.

### Verifikasi
- Setelah disk ticker fire (5 menit), cek DB langsung
- Pastikan `disk_usage_bytes > 0` untuk app yang punya data
