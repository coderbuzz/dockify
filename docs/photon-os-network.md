# Photon OS Network — Internal Adapter (ESXi)

## Setup static IP di interface baru

### 1. Tambah adapter via ESXi

VM Settings → Add Network Adapter → pilih **Internal** network → OK.

### 2. Cek nama interface

```bash
ip link show
# atau
networkctl list
```

Biasanya `eth1`, bisa juga `ens192`, `ens224`, dll.

### 3. Set IP permanen (systemd-networkd)

```bash
sudo tee /etc/systemd/network/99-eth1.network << 'EOF'
[Match]
Name=eth1

[Network]
Address=10.0.0.3/24
EOF
```

**Penting:** pastikan permission file benar (systemd-networkd berjalan sebagai user `systemd-network`):

```bash
sudo chown root:systemd-network /etc/systemd/network/99-eth1.network
sudo chmod 644 /etc/systemd/network/99-eth1.network
```

### 4. Apply

```bash
sudo systemctl restart systemd-networkd
```

### 5. Verifikasi

```bash
sudo networkctl status eth1
ip addr show eth1
ping 10.0.0.2
```

### 6. Jika tetap unmanaged / IP tidak muncul

Coba assign manual dulu untuk test koneksi:

```bash
sudo ip addr add 10.0.0.3/24 dev eth1
sudo ip link set eth1 up
```

Kalau berhasil, kemungkinan masalah:
- **Permission file** — pastikan `chown` + `chmod` seperti di atas
- **Interface DOWN** — `sudo ip link set eth1 up`
- **Konflik Match** — file `.network` lain mungkin match alternative names (cek `ip link show eth1` lihat `altname`). Rename file jadi `99-*.network` biar prioritas tinggi.

## Enumerate interfaces

Lihat alternative names jika nama interface tidak jelas:

```bash
ip link show | grep altname
```
