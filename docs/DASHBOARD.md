# Model observatory

`induction dashboard generate` reads `.sessions/` and writes `data/dashboard/session_metrics.json` and a self-contained `data/dashboard/dashboard.html`. Open the HTML with `file://`; it requires no server or network connection. The dashboard is historical, not live monitoring. `.dashboardignore` can list exact model IDs to exclude, one per line.

The seven views answer distinct questions:

| View | Main question | Explorers |
| --- | --- | --- |
| Overview | What was collected, and is it trustworthy? | Fleet Command Center; Reliability & Failure; Data Quality & Coverage |
| Performance | How fast were observations, and how does workload relate to speed? | Model Performance; Context Pressure; Inference Time Breakdown; Evolution |
| Response behavior | What appeared in responses? | Inference Anatomy |
| Models | What measurements and configurations exist? | Comparison Matrix; Performance Fingerprint; Runtime |
| Recommendations | Which selected models lead on recorded workload signals? | Six visible workload blocks with leader and next contender comparisons |
| Playground | How do the selected models compare across five dimensions? | Fleet Foundry and Signal Loom |
| Variables | Which direct and inferred fields are available? | Existing Variables reference |

The global model picker starts with every model selected. Search filters picker options without changing selections. **Clear** produces an explicit empty state; **Select all** restores the fleet. The selected IDs and current view are saved in the URL fragment, for example `#performance&models=model-a%2Cmodel-b`. Unknown IDs are ignored. The Variables view remains fleet-wide and uses its existing calculations.

Click a model, bar, matrix row, or scatter point (or focus it and press Enter) to open the Metrics Inspector. Its tabs contain values, source observations, runtime configuration, and optional raw JSON. Escape closes the inspector. The theme button persists the light or dark theme in browser storage when available.

## Schema and provenance

Schema version 2 keeps observations grouped by `model_id`. Each observation has its original `session.id`, `session.snapshot_index`, `collected_at`, response counts and finish reason, throughput, token counters, tool indicators, and a `runtime_config_id`. `runtime_configs` holds each distinct model/runtime configuration once, including context size, slots, build, alias, modalities, and generation settings. The config ID is a stable SHA-256-derived key over model ID and configuration JSON. Local model paths and chat templates are excluded from the analytical export. Schema-v1 HTML data remains readable: the dashboard uses inline observation runtime fields and projects positive legacy load durations into distinct display events. Regenerate older exports to receive the v2 catalog and source counts.

`source.session_files` and `source.sessions_loaded` count files loaded. `source.unique_session_ids` counts their unique IDs. `source.observed_session_ids` counts distinct IDs represented by included snapshots. `source.sessions_without_observations` counts loaded sessions with no included snapshots. `snapshots_seen`, `snapshots_included`, and `snapshots_skipped` are separate counts. For the currently saved dataset, 60 files load, 50 sessions contain observations, and 10 do not; all 222 snapshots are included. Model-level session counts must not be summed to calculate fleet-wide unique sessions because a session can use multiple models.

Metrics labeled **observed** come from saved snapshots. The shared dashboard engine calculates min, median, mean, p90 and p95 from valid per-snapshot values; its sample count is the number of valid snapshots and its session count is distinct contributing session IDs. Null or malformed numeric values are unavailable, while recorded zero stays zero. The coverage grid displays valid/total snapshot counts. Configured context size is a limit, not a verified ability. Captured reasoning words are text counts, not token costs or reasoning quality. `finish_reason=stop` means normal termination, not task success. A `length` finish indicates truncation risk; an empty visible response is shown separately. A zero cached-token counter does not establish that caching was disabled.

The time breakdown uses **estimated scenarios**: selected prompt token budget / median prompt tok/s, and selected output token budget / median generation tok/s. These estimates are not request wall time or actual prefill/generation measurements. Measured mode appears separately and requires recorded prompt and generation durations plus completion tokens in the same snapshot; absent paired telemetry stays unavailable. Including load adds the median of distinct recorded load events as a separate segment, only when available. The context scatter and timeline use actual snapshot pairs and timestamps, with no fitted performance curve. Workload differences can explain observed speed differences.

### Load timing

The application already records `ModelSnapshot.ModelLoadTime`. `ensureModelLoaded` obtains it from a changed runtime lifecycle operation or a pending explicit model load. The duration uses the client's monotonic `time.Since` clock, in nanoseconds in persisted Go data and milliseconds in the dashboard. It covers the client-observed transition, including lifecycle requests and polling until loaded; it is not a server hardware-only load time. It is zero when already loaded or unreported. Tool loops may copy a first-turn duration onto a final snapshot. The v2 exporter conservatively groups repeated equal positive durations for the same session and model into one `load_events` entry, with a source, runtime config reference, and associated snapshot timestamp. The exact transition completion time and cold-versus-warm classification were not persisted, so `kind` is `unknown`; no historical cold-start claims are made. The current dataset contains no positive recorded load durations. Actual completion tokens, per-request elapsed timings, and TTFT are retained only if existing response telemetry contains them; the exporter does not infer missing values.

### Recommendations

Recommendations show all six workloads at once. Speed-first uses median generation throughput. Long-context uses median observed prompt size when a context configuration is recorded. Reasoning uses the median share of captured response words attributed to reasoning. Vision uses generation throughput in image-input snapshots. Structured extraction uses generation throughput in JSON-format output snapshots. Tool-enabled agents use the share of snapshots with tool availability or a recorded tool call. Each block shows the leader and next contender, if one exists, with concentric value rings and paired bars for the workload metric, matching snapshots, independent sessions, and normal termination. The blocks animate into view on scroll, while reduced-motion settings show them immediately. Models with no matching observation are omitted from that workload. These operational rankings do not measure task correctness, image understanding, schema validity, or successful tool use.

The v2 projection omits raw transcript and reasoning text. Raw sessions remain in `.sessions/`. The HTML embeds the analytical JSON; take care when sharing it because session IDs, titles, model IDs, and tool names can reveal local context.

See the [migration manifest](DASHBOARD-MIGRATION.md) for legacy component mapping and validation limits.

## Browser regression check

With Playwright and its Chromium browser available, run:

```bash
node scripts/dashboard-browser-check.mjs data/dashboard/dashboard.html /tmp/induction-dashboard-screenshots
```

The optional second argument saves screenshots at every required CSS width in both themes. The check opens each configured view at every required width, asserts no page-level overflow or console errors, and checks workload blocks, empty filters, keyboard inspector access, and Variables isolation. `INDUCTION_PLAYWRIGHT_PACKAGE` and `INDUCTION_CHROME_PATH` can point to an existing Playwright installation and browser binary.
