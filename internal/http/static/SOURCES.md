# Vendored browser assets

Served by Dockify at `/static/*` (embedded with `//go:embed`). To upgrade, download
the new version from the same place, update the URL and sha256 here and the file
names in `templates/layout.html` if they change.

| File | Source | sha256 |
|---|---|---|
| htmx.min.js | https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js | e209dda5c8235479f3166defc7750e1dbcd5a5c1808b7792fc2e6733768fb447 |
| xterm.min.js | https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.min.js | fc1dd31b221e3e5f929486e07a80b477a8aaf9dce2b4f9c3ffe7dd25f370655d |
| xterm.min.css | https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.min.css | 64ee6c4db69b4224d3362aced0fd4cdd620e0e60b3d01566450ae2d4b9e81849 |
| xterm-addon-fit.min.js | https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.min.js | c78e5795c6487acdf24ab436798d4a9cec3848101b281bc65b061f39db714be1 |
