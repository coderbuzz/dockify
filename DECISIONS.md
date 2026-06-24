# Architectural Decision Records

## ADR-001: Build own platform vs use Coolify

**Decision:** Build Dockify from scratch.

**Context:** We needed a platform to manage VMs and deploy Dockerized apps with auto HTTPS, Cloudflare DNS, and Git CI/CD. Coolify exists as a mature solution (57k stars).

**Rationale:**
- Full control over architecture and features
- No dependency on external project lifecycle
- Leaner — only build what we need (no 280 one-click services, no multi-tenancy, no teams)
- Single binary vs Coolify's multi-service (PostgreSQL, SvelteKit, agent)
- Deeper understanding = easier debugging
- Can customize exactly to our workflow

**Consequences:**
- 2-3 week build time vs 1-hour Coolify install
- We own bugs and maintenance

## ADR-002: Caddy as reverse proxy

**Decision:** Use Caddy, not Traefik or Nginx.

**Context:** Existing setup uses Traefik with Cloudflare DNS challenge. We evaluated Caddy, Traefik, Nginx, and Cloudflare Tunnel.

**Why not Cloudflare Tunnel:**
- Adds 10-50ms latency per roundtrip
- Not designed for database protocols (MongoDB, PostgreSQL)
- WebSocket support exists but not optimized for thousands of persistent connections
- Docker container ↔ CF Tunnel setup is complex for non-HTTP

**Why Caddy over Traefik:**
| | Traefik | Caddy |
|---|---|---|
| RAM idle | ~50-100MB | ~15-25MB |
| Auto HTTPS | Needs provider config | Zero config |
| Route changes | Docker labels | Admin API (POST JSON) |
| Config syntax | YAML + labels | Caddyfile / JSON API |
| WebSocket | Native | Native |

**Why Caddy over Nginx:**
- Nginx has no built-in Let's Encrypt (needs certbot)
- Nginx config is static (needs reload on changes)
- Caddy's Admin API allows dynamic route injection without restart
- Caddy is auto-HTTPS by default

**Consequences:**
- Traefik will be decommissioned after migration
- Apps route via Caddy Admin API, not Docker labels

## ADR-003: Reverse proxy per worker VM

**Decision:** Each worker VM runs its own Caddy instance.

**Context:** Alternative would be a single central Caddy routing to all VMs.

**Rationale:**
- No single point of failure for routing
- Lower latency (no extra hop)
- Each VM handles its own SSL termination
- Worker VMs are self-contained
- DDoS/load is distributed

**Consequences:**
- Caddy Admin API must be bound to localhost only (security)
- Controller manages Caddy config remotely via SSH tunnel + Admin API

## ADR-004: Database engines deployed without public port

**Decision:** MongoDB, PostgreSQL, and other databases are deployed on internal Docker networks only — no port mapping to host.

**Rationale:**
- Database engines should never be exposed to the internet
- Backend apps connect via Docker internal network (container name resolution)
- Reduces attack surface
- Cloudflare Tunnel/CDN is irrelevant for database traffic

**Consequences:**
- Direct DB access only via SSH tunnel to VM (for admin/debugging)
- Backend apps must be on same Docker network as databases

## ADR-005: WebSocket — direct via Caddy, not through Cloudflare Tunnel

**Decision:** WebSocket traffic goes: Internet → Cloudflare DNS (orange cloud optional) → VM public IP → Caddy → WebSocket backend.

**Rationale:**
- Thousands of persistent WebSocket connections need minimal latency
- Cloudflare Tunnel adds unnecessary hop
- Caddy handles WebSocket upgrade natively
- Cloudflare DNS proxy (orange cloud) supports WebSocket passthrough if DDoS protection needed

## ADR-006: Single monorepo

**Decision:** One repo `coderbuzz/dockify` for everything.

**Rationale:**
- Go binary + embedded web UI = single artifact
- No NPM, no Webpack, no separate frontend build
- HTMX + Go templates = frontend is just templates in the binary
- No coordination overhead between repos
- Pattern used by Caddy, Traefik, and most Go projects

## ADR-007: Go + SQLite + HTMX stack

**Decision:** Go backend, SQLite database, HTMX + Pico CSS frontend.

**Why Go:**
- Single binary deployment (no runtime, no VM setup)
- Excellent SSH library (`golang.org/x/crypto/ssh`)
- Fast, low resource usage
- Great stdlib (net/http, embed, html/template)

**Why SQLite:**
- Embedded, zero ops (no PostgreSQL to manage)
- Pure Go driver (`modernc.org/sqlite`) = no CGo, cross-compile easy
- Sufficient for VM and app state (not storing metrics/time-series)
- Single file backup

**Why HTMX + Pico CSS:**
- No JavaScript framework build step
- Templates embedded in Go binary via `embed.FS`
- HTMX handles partial page updates without SPA complexity
- Pico CSS is minimal class-less styling

## ADR-008: Project deployment config stays in project repos

**Decision:** Dockify is generic. Per-project deployment config (docker-compose.yml, env, domain mappings) stays in project repos (e.g., `amg-id/tmx-devops`).

**Rationale:**
- Separation of concerns: platform vs project
- Projects own their deployment config
- Dockify reads config from project repo via git
- Same pattern as Coolify (services defined in app repos)

**File convention:** `.dockify.hcl` or `.dockify.yml` in project root defines apps.
