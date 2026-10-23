# Native inventory table — visual evidence

The screenshots use an isolated four-stack/three-container fixture at a 900px viewport height. They are not screenshots of a refreshed demo. The running demo at port 18881 was inspected read-only and was not rebuilt or restarted.

The before screenshots serve the supplied comparison snapshot from a separate temporary Vite root. Both DM Sans and IBM Plex Mono were confirmed loaded; no font requests failed. The after screenshots are produced by the native-table geometry browser tests. Fixture observation timestamps naturally differ between captures.

## Measured presentation

| Measurement | Before | After |
| --- | --- | --- |
| Desktop document height, 1440px wide | 1474px | 900px |
| Desktop stack header rows | Separate multi-line groups | 48px each |
| Desktop container rows | Nested multi-line content | 44px each |
| Shared column alignment | No shared table columns | Exact matching header/cell x positions and widths |
| Document width at 320px | 320px | 320px |

After typography: names are DM Sans, 13px, weight 500 (stack names 600); technical references/revisions/digests use IBM Plex Mono, 12px. Deployment/runtime statuses have text plus Lucide icons, with distinct green/red/amber/blue/neutral colors. Reduced-motion checks report no icon animation.

The mobile table deliberately scrolls horizontally inside its own labeled, focusable region. The second mobile after image shows the right-hand action columns, including the 44px Review action and digest-copy targets. It does not imply all five columns are simultaneously visible at 320px.

Raw measurements: `before-geometry-1440.json`, `before-geometry-320.json`, `geometry-1440.json`, `geometry-320.json`.

## Verification

- `frontend`: `bun run lint`, `bun run test` (223 tests), `bun run build`.
- `tests/e2e`: `bun run test:frontend` (32 browser cases).
- `tests/e2e`: `bun run test:frontend --grep 'native inventory|compact global'` (4 cases; final geometry/mobile action evidence, including adjacent preview-row heights).
- `tests/e2e`: `bun run test` (1 complete real-Docker/DinD browser lifecycle, not the full backend integration suite).

The browser suites retain concrete lifecycle, approval/revision rejection, uncertain-request, reconnect, pinned-identity, stale-runtime, scoped navigation/logs, bounded preview/search, focus/Back, and light/dark reflow assertions. Error diagnostics are asserted through the operation inspector rather than requiring the full error essay in every default inventory row.

Limitations: no native screen-reader testing and no user-validated usability claim. Demo frontend refresh and delivered-demo geometry verification remain the lead's separate task after Tester approval. No backend or service/store contract was changed by this presentation follow-up.
