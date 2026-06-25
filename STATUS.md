# Status Dockify — 25 Jun 2026

## Yang sudah berfungsi
- ✅ Controller: Linux Debian 12, binary + native Caddy (mode 3)
- ✅ Domain: dockify.amg.id (https)
- ✅ Worker register + init (Docker, network, Caddy)
- ✅ Login page admin
- ✅ Route inject via Admin API (fixed: initialize routes array sebelum POST)
- ✅ HTTPS di worker port 443
- ✅ Multi-instance webhook deploy (1 push → redeploy semua app)
- ✅ Global webhook secret via Settings page
- ✅ Settings page (view/copy/roll webhook secret)

## Fix terbaru (25 Jun)
1. **Caddy route injection** — POST `[]` dulu ke `/routes` untuk inisialisasi array (fresh Caddy routes null)
2. **Multi-instance webhook** — `DeployByGit` loop semua app yang match repo+branch, bukan cuma 1
3. **Global webhook secret** — pindah dari per-app ke global di table settings. Hapus kolom `webhook_secret` dari apps. Settings page di UI untuk copy/roll secret.

## Rencana
- Setup CI GitHub Action utk auto-deploy via webhook
- Test multi-instance webhook dengan kvs-server-app
