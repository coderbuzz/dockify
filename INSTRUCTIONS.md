# Status Fix Caddy Route

## Masalah
Route app ditambahkan ke `/routes` (akhir array) setelah file server default Caddy. Akibatnya domain app tidak terlayani (halaman "Caddy works!").

## Fix (sudah dijalankan)
Route di-inject ke posisi `/routes/0` (sebelum file server default).

```
delete /id/dockify-kv-amg-id
post /routes/0 → route baru
```

## Verifikasi
```bash
# Dari luar VM
curl -v https://kv.amg.id/
# Dari controller VM
curl -k https://kv.amg.id/
```

> Jangan test localhost:2019 — itu Admin API, bukan HTTP server.

## Permanent Fix
Kode `postRoute()` sudah diubah dari `/routes` menjadi `/routes/0`. Setelah CI selesai, update controller:

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/update.sh | bash
```

Lalu redeploy app dari UI.
