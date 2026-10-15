# Perf sebelum S7 (main setelah S6, build mock, `node tests/e2e/ui.mjs`)

# Dockify UI report

Base: http://127.0.0.1:18091 · 2026-10-15T12:46:28.151Z · runs=5

## Navigation (CPU 4x, +80 ms per request, median)

| page | DOMContentLoaded ms | load ms |
| --- | --- | --- |
| dashboard | 327 | 400 |
| servers | 324 | 389 |
| apps | 326 | 380 |
| app-detail | 338 | 397 |


## Transfer (cold, on the wire) and DOM nodes @390

| page | KB | requests | DOM nodes | nodes/row |
| --- | --- | --- | --- | --- |
| dashboard | 467 | 11 | 253 | 14 |
| servers | 461 | 11 | 146 | 14 |
| apps | 463 | 11 | 174 | 12 |
| app-detail | 473 | 12 | 268 | 7 |
| app-draft | - | - | 185 | 0 |
| server-detail | - | - | 339 | 11 |
| apps-add | - | - | 299 | 0 |
| settings | - | - | 121 | 0 |


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

