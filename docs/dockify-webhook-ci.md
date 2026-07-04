# Dockify Webhook CI Setup

AI prompt untuk setup GitHub Actions deploy via Dockify webhook di repo app.

---

## Cara Kerja

```
Push ke main → CI build Docker image → push ke registry → notify Dockify via webhook
                                                                  ↓
                                        Dockify redeploy semua app yang pakai repo + branch ini
                                        (docker compose pull + up -d + inject Caddy route)
```

Satu webhook bisa trigger redeploy **semua instance** app yang menggunakan repo dan branch yang sama. Dockify melakukan:
1. SSH ke worker → `docker compose pull && docker compose up -d --remove-orphans`
2. Inject ulang Caddy route (domain → container:port)
3. Record deployment history (success/failed)

---

## Setup

### 1. Di Dockify — daftarkan app dengan git repo
Saat create/edit app, isi field:
- **Git Repo:** clone URL, misal `https://github.com/user/repo.git`
- **Git Branch:** branch yang ditrigger, misal `main`
- **Image:** gunakan tag `latest` atau `main` agar CI update otomatis terpakai

Webhook secret bersifat **global** (1 secret untuk semua app). Copy dari **Settings page** di Dockify UI.

### 2. Di GitHub repo — tambah secret
**Settings → Secrets and variables → Actions → New repository secret:**
- Name: `DOCKIFY_WEBHOOK_SECRET`
- Value: paste webhook secret dari Dockify

### 3. Tambah workflow file
Buat file `.github/workflows/docker-deploy.yml`:

```yaml
name: Docker Build & Deploy

on:
  push:
    branches: [main]
    tags: ["v*"]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Login to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=branch
            type=ref,event=tag
            type=sha,prefix=,format=short
            type=raw,value=latest,enable=${{ github.ref == 'refs/heads/main' }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}

      - name: Notify Dockify
        run: |
          REPO="https://github.com/${{ github.repository }}.git"
          BRANCH="${GITHUB_REF#refs/heads/}"
          COMMIT="${{ github.sha }}"
          SECRET="${{ secrets.DOCKIFY_WEBHOOK_SECRET }}"

          PAYLOAD=$(cat <<EOF
          {"ref":"refs/heads/${BRANCH}","after":"${COMMIT}","repository":{"clone_url":"${REPO}"}}
          EOF
          )

          SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/^.* //')

          curl -s -o /dev/null -w "%{http_code}" \
            -X POST https://dockify.amg.id/api/webhook/github \
            -H "Content-Type: application/json" \
            -H "X-GitHub-Event: push" \
            -H "X-Hub-Signature-256: sha256=${SIGNATURE}" \
            -d "$PAYLOAD"
```

### 4. Konvensi Docker image tag
App di Dockify sebaiknya menggunakan tag yang konsisten dengan CI:
- Gunakan `image: ghcr.io/user/repo:latest` di compose → CI push `latest` saat push ke main
- Atau `image: ghcr.io/user/repo:main` → image tagged per branch name

### 5. Multi-instance
Satu repo bisa dipakai oleh banyak app di Dockify (beda domain, beda worker).
Webhook akan auto-redeploy **semua instance** yang memiliki `git_repo` + `git_branch` yang sama.

---

## Environment Variables untuk App

Jika app butuh env vars (seperti `ACCESS_TOKEN`, `DOMAIN`), set langsung di Dockify compose editor
saat create/edit app. Tidak perlu `.env` di repo atau CI — semua config via Dockify UI.

---

## Debug

- **Webhook return 401:** secret tidak cocok. Cek webhook secret di Settings page Dockify vs `DOCKIFY_WEBHOOK_SECRET` di GitHub secrets.
- **App tidak redeploy:** cek `git_repo` dan `git_branch` di Dockify app detail.
- **Docker build gagal:** cek GitHub Actions log.
