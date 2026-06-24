# Fix Caddy Route Matching

## Masalah
Caddy di worker VM jalan tanpa Caddyfile (default mode). Route yang diinject via Admin API tidak match meskipun route terdaftar. Akibatnya domain app tidak terlayani (halaman "Caddy works!").

## Fix untuk Worker Existing

Jalankan di **WORKER VM** untuk restart Caddy dengan Caddyfile:

```bash
mkdir -p /opt/dockify/caddy
cat > /opt/dockify/caddy/Caddyfile << 'EOF'
:80, :443 {
}
EOF
docker rm -f caddy
docker run -d --name caddy --network dockify \
  -p 80:80 -p 443:443 -p 127.0.0.1:2019:2019 \
  -v caddy_data:/data \
  -v /opt/dockify/caddy/Caddyfile:/etc/caddy/Caddyfile:ro \
  --restart unless-stopped caddy:latest
```

Setelah Caddy restart, **redeploy app dari UI Dockify**.

## Permanent Fix (sudah di kode)

InitWorker sekarang deploy Caddy dengan Caddyfile `:80, :443 {}` langsung. Worker baru akan otomatis pakai config ini.

## Verifikasi

```bash
# Cek apakah route terdaftar
docker exec caddy curl -s http://localhost:2019/id/dockify-kv-amg-id

# Test akses dari luar
curl -s https://kv.amg.id/health
```
