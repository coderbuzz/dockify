# Status Dockify — 25 Jun 2026

## Yang sudah berfungsi
- ✅ Controller: Linux Debian 12, binary + native Caddy (mode 3)
- ✅ Domain: dockify.amg.id (https)
- ✅ Worker register + init (Docker, network, Caddy)
- ✅ Login page admin
- ✅ Route inject via Admin API
- ✅ HTTPS di worker port 443 (manual DELETE+PUT listen)
- ✅ Route posisi 0 (manual POST ke /routes/0)

## Yang belum
- ❌ Route inject via `postRoute()` selalu error 500 `cannot unmarshal object...`
  - Penyebab: Admin API Caddy tidak terima format JSON route setelah listen diubah
  - Coba tanpa Caddyfile (revert commit Caddyfile)
- ❌ Caddyfile approach juga gagal (route inject error sama)
- ❌ Route tidak match untuk domain `kv.amg.id` (selalu "Caddy works!")

## Ringkasan error
1. `routes` → 500 unmarshal error (POST route setelah listen diubah)
2. `routes/0` → 500 same error
3. Caddyfile → same error

## Rencana besok
1. Jalankan dari worker VM
2. Debug langsung SSH ke Caddy Admin API
3. Cek response body curl langsung
4. Fix format JSON route biar sesuai Caddy v2 API spec
