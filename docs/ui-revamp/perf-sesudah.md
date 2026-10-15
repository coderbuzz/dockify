# Perf sesudah S7 (cabang feat/ui-s7-measure, build mock, `node tests/e2e/ui.mjs`)

# Dockify UI report

Base: http://127.0.0.1:18090 · 2026-10-15T12:48:49.209Z · runs=5

## Navigation (CPU 4x, +80 ms per request, median)

| page | DOMContentLoaded ms | load ms |
| --- | --- | --- |
| dashboard | 300 | 315 |
| servers | 301 | 311 |
| apps | 301 | 309 |
| app-detail | 328 | 336 |


## Transfer (cold, on the wire) and DOM nodes @390

| page | KB | requests | DOM nodes | nodes/row |
| --- | --- | --- | --- | --- |
| dashboard | 126 | 7 | 248 | 14 |
| servers | 120 | 7 | 141 | 14 |
| apps | 122 | 7 | 169 | 12 |
| app-detail | 192 | 10 | 266 | 7 |
| app-draft | - | - | 183 | 0 |
| server-detail | - | - | 337 | 11 |
| apps-add | - | - | 294 | 0 |
| settings | - | - | 116 | 0 |


## Overflow @360/390

none (0)


## Contrast (WCAG, 19 pairs)

| palette | theme | pass |
| --- | --- | --- |
| indigo | light | 19/19 |
| indigo | dark | 19/19 |
| navy | light | 19/19 |
| navy | dark | 19/19 |
| teal | light | 19/19 |
| teal | dark | 19/19 |


## A11y quick

| page | unnamed controls | dialogs w/o label | first Tab focus |
| --- | --- | --- | --- |
| dashboard | 0 | 0 | ok |
| servers | 0 | 0 | ok |
| apps | 0 | 0 | ok |
| app-detail | 0 | 0 | ok |
| app-draft | 0 | 0 | ok |
| server-detail | 0 | 0 | ok |
| apps-add | 0 | 0 | ok |
| settings | 0 | 0 | ok |


## Result

PASS

