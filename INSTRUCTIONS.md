## Fix: Caddy Route Injection Position

Masalah: route app ditambahkan setelah file server default Caddy, jadi domain app tidak terlayani (halaman "Caddy works!").

### Manual fix (jalankan di WORKER VM)

```bash
# 1. Hapus route lama (kalau ada)
docker exec caddy curl -s -X DELETE http://localhost:2019/id/dockify-kv-amg-id 2>/dev/null

# 2. Tambah route baru di posisi 0 (sebelum file server default)
docker exec caddy curl -s -X POST http://localhost:2019/config/apps/http/servers/srv0/routes/0 \
  -H 'Content-Type: application/json' \
  -d '{"@id":"dockify-kv-amg-id","match":[{"host":["kv.amg.id"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"app:3000"}]}]}'
```

### Verifikasi

```bash
# Cek apakah route terdaftar
docker exec caddy curl -s http://localhost:2019/id/dockify-kv-amg-id

# Test akses app via curl
docker exec caddy curl -v http://localhost:2019/
```

### Permanent fix (menunggu CI release)

Kode di `internal/caddy/client.go` sudah diubah dari `/routes` menjadi `/routes/0` (commit d35c261). Setelah CI build selesai, update binary controller:

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/update.sh | bash
```

Lalu redeploy app dari UI.
