# Dashboard metrics

Use the dashboard generator when you want a server-free HTML view of telemetry
and responses saved by previous sessions. It reads persisted session data and
does not perform inference.

Generate a server-free metrics projection from saved and unsaved sessions:

```bash
induction dashboard generate
```

The source is `.sessions/`. Outputs are
`data/dashboard/session_metrics.json` and the self-contained
`data/dashboard/dashboard.html`. Raw transcripts, reasoning, responses,
properties, metrics, and slot payloads remain in the session files.

To exclude models from the generated dashboard, create `.dashboardignore` in
the repository root with one exact model ID per line. Blank lines and lines
starting with `#` are ignored. Matching model snapshots and evaluation results
are omitted from both dashboard artifacts. If the file is absent, no models
are excluded.

See the [CLI reference](CLI-REFERENCE.md#server-and-runtime-json-output) for
related command options.

The generated dashboard includes a **Fingerprints** view. It derives model-level
telemetry from saved observations, including median load, prompt, and generation
metrics; mean response-content metrics; thought density; output yield; cold-start
sprint; throughput blend; and observed image/structured-output workload mix. It
also includes interactive constellation charts, compound metric definitions,
deterministic narrative insights, and a model lineup table.

Fingerprint values are workload observations rather than controlled benchmarks.
Missing telemetry is omitted from calculations. Models with fewer than five
snapshots are marked as small-sample results; comparisons with at least ten
snapshots receive the high-sample label. The model search field filters this view
along with the rest of the dashboard.
