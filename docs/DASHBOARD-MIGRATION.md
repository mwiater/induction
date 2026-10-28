# Dashboard refactor migration manifest

The production exporter is `dashboard.go`; `dashboard.template.html` is the source page. `.sessions` snapshots pass through `dashboardObservation`, model grouping, JSON encoding, HTML embedding, a shared browser metric engine, then the six views. The old template used inline CSS/JS and remote libraries. The new template uses inline CSS/JS without remote dependencies. Variables markup and calculations were carried forward; schema-v2 configs are rehydrated in memory for its existing reader.

| Legacy element / user question | Metrics retained | Decision and target | Verification |
| --- | --- | --- | --- |
| `experimental04Timeline`, fleet KPIs, unexpected insights / what happened? | Model, session and snapshot counts, timestamps | Merge into Fleet Command Center; remove duplicate KPIs and narrative cards | Fleet count and timeline DOM checks |
| Dataset health / can I trust this? | Source counts, field availability | Expand into reliability and coverage explorers | Exporter and DOM checks |
| `experimental02Loom`, speed charts, fastest-speed KPIs / which is faster? | Snapshot prompt and generation tok/s | Replace with Model Performance Explorer | Shared medians and inspector |
| `experimental01LatencyOrigami`, startup scatter / where does time go? | Throughput, token budgets, load events | Replace with labeled scenario time breakdown and actual context scatter | Missing load remains unavailable |
| `fingerprintStartupPrompt`, context and stability panels / does context matter? | Prompt tokens, context config, observed speed | Merge into Context Pressure and Evolution | Observed point count |
| `experimental01Economy`, `experimental01Forge`, response cards / what did model emit? | Visible and captured reasoning words, finish reason | Merge into Inference Anatomy | Source response counters |
| `experimental01Prism`, capability genome, profile cards / how do models compare? | Runtime config, throughput, outcome counts | Replace with Matrix, Fingerprint, Runtime Explorer; remove genome | No genome DOM ID in template |
| `recommendationMap`, `modelRecommendations`, `experimental05Network` / what should I choose? | Observed speed, context and task eval scores | Replace with one workload selector and abstaining evidence card | Eligibility and empty state checks |
| `variables` / what raw fields exist? | Existing direct and inferred calculations | Retain markup and calculation logic | Variables view DOM check |

Historical schema-v1 JSON remains readable by the page; regenerate to obtain version 2 metadata, catalog references and load events. Old exported HTML files remain standalone snapshots and are not rewritten. The removed cards and scripts were production template code, not exported-file patches.

Real-browser validation used the committed `scripts/dashboard-browser-check.mjs` with the installed Playwright Chromium driver against the generated `file://` dashboard. Every view was opened in dark and light themes at 320, 360, 375, 390, 430, 600, 768, 1024, 1280, 1440, and 1920 CSS pixels; no page-level horizontal overflow or JavaScript console errors were observed. All 132 view/width/theme screenshots were captured. Reduced-motion mode was used for the layout matrix. Phone and desktop screenshots were visually inspected and the tall Response, Performance, and Runtime sections were compacted. Manual physical touch and screen-reader testing remains separate from this automated pass.
