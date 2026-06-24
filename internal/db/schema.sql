CREATE TABLE IF NOT EXISTS servers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    host        TEXT NOT NULL,
    port        INTEGER DEFAULT 22,
    user        TEXT DEFAULT 'root',
    ssh_key     TEXT NOT NULL,
    status      TEXT DEFAULT 'pending',
    cpu_cores   INTEGER,
    ram_mb      INTEGER,
    disk_gb     INTEGER,
    cpu_usage   REAL,
    ram_usage   REAL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS apps (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    server_id   INTEGER REFERENCES servers(id),
    domain      TEXT NOT NULL,
    port        INTEGER NOT NULL,
    compose     TEXT NOT NULL,
    git_repo    TEXT,
    git_branch  TEXT DEFAULT 'main',
    auth_user   TEXT DEFAULT '',
    auth_pass   TEXT DEFAULT '',
    webhook_secret TEXT DEFAULT '',
    status      TEXT DEFAULT 'created',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deployments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id          INTEGER REFERENCES apps(id),
    server_id       INTEGER REFERENCES servers(id),
    status          TEXT,
    log             TEXT,
    commit_sha      TEXT,
    compose_snapshot TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS routes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id      INTEGER REFERENCES apps(id),
    server_id   INTEGER REFERENCES servers(id),
    domain      TEXT NOT NULL,
    target      TEXT NOT NULL,
    status      TEXT DEFAULT 'active',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS dns_records (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id      INTEGER REFERENCES apps(id),
    server_id   INTEGER REFERENCES servers(id),
    zone_id     TEXT NOT NULL,
    record_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT DEFAULT 'A',
    content     TEXT NOT NULL,
    proxied     INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
