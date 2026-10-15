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
| htmx-ext-preload.min.js | https://unpkg.com/htmx-ext-preload@2.1.2/dist/preload.min.js | 2ac8ccf77deee4f8e87366d6b606085eed755b02513746ead7573395e717d26e |
| htmx-ext-head-support.min.js | https://unpkg.com/htmx-ext-head-support@2.0.5/dist/head-support.min.js | 86e59ae1048bf02fb31a5842b4b559a1058b4c3ff190c258530ecb94f9fa6d56 |

`icon-192.png` / `icon-512.png`: the "D" logo on `accent-fill` indigo (#4f46e5), rendered from SVG with Playwright; `manifest.webmanifest` and `sw.js` are first-party.
